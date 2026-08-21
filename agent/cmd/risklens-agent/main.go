package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"risklens/agent/internal/app"
	cfgmanager "risklens/agent/internal/configuration/manager"
	"risklens/agent/internal/configuration/model"
	cfgstorage "risklens/agent/internal/configuration/storage"
	"risklens/agent/internal/identity/generator"
	idmanager "risklens/agent/internal/identity/manager"
	idstorage "risklens/agent/internal/identity/storage"
	"risklens/agent/internal/logging"
	"risklens/agent/internal/modules/activity"
	"risklens/agent/internal/modules/browseractivity"
	"risklens/agent/internal/modules/device"
	"risklens/agent/internal/modules/deviceupload"
	"risklens/agent/internal/modules/heartbeat"
	"risklens/agent/internal/modules/interfaces"
	"risklens/agent/internal/modules/registration"
	"risklens/agent/internal/modules/software"
	"risklens/agent/internal/modules/users"
	"risklens/agent/internal/service"
	"risklens/agent/internal/version"
)

const (
	exitOK            = 0
	exitUsage         = 2
	exitConfigMissing = 10
	exitConfigInvalid = 11
	exitLogger        = 20
	exitIdentity      = 21
	exitService       = 30
	exitApp           = 40
	exitShutdown      = 50
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 && args[0] == "--activity-foreground" {
		return activity.RunForegroundHelper()
	}
	if browseractivity.IsNativeHostInvocation(args) {
		return browseractivity.RunNativeHost()
	}
	bootstrap := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	isService, err := service.IsWindowsService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "service detection failed: %v\n", err)
		return exitService
	}

	var (
		configPath string
		console    bool
	)

	if isService {
		configPath = os.Getenv(model.EnvConfig)
	} else {
		fs := flag.NewFlagSet("RiskLensAgent", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		configFlag := fs.String("config", "", "path to agent.yaml")
		consoleFlag := fs.Bool("console", false, "run in console mode")
		showVersion := fs.Bool("version", false, "print version and exit")
		if err := fs.Parse(args); err != nil {
			return exitUsage
		}
		if *showVersion {
			fmt.Println(version.String())
			return exitOK
		}
		configPath = *configFlag
		console = *consoleFlag
	}

	store, err := cfgstorage.NewYAMLStore(cfgmanager.ResolvePath(configPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		return exitConfigInvalid
	}
	cfgMgr, err := cfgmanager.New(store, bootstrap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		return exitConfigInvalid
	}
	cfg, err := cfgMgr.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		if errors.Is(err, cfgstorage.ErrNotFound) {
			return exitConfigMissing
		}
		return exitConfigInvalid
	}

	useConsole := console || !isService
	log, err := logging.New(logging.Options{
		Console:     useConsole,
		Level:       logging.ParseLevel(cfg.Logging.Level),
		FilePath:    cfg.Logging.File,
		ServiceName: cfg.General.ServiceName,
		Version:     version.Version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		return exitLogger
	}
	defer log.Close()

	logger := log.Logger.With("component", "main")
	logger.Info("Version", "commit", version.Commit, "build_time", version.BuildTime)
	logger.Info("Configuration loaded", "path", cfg.Path)

	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	if err := cfgMgr.Watch(watchCtx); err != nil {
		logger.Warn("configuration watch not started", "error", err)
	}

	displayName := cfg.General.DisplayName
	if cfg.Identity.DisplayName != "" {
		displayName = cfg.Identity.DisplayName
	}
	idPath := cfg.Storage.IdentityFile
	if idPath == "" {
		idPath = idstorage.DefaultPath()
	}
	idStore, err := idstorage.NewFileStore(idPath)
	if err != nil {
		logger.Error("identity store init failed", "error", err)
		return exitIdentity
	}
	idMgr, err := idmanager.New(idStore, generator.CollectInfo(version.Version, displayName), log.Logger)
	if err != nil {
		logger.Error("identity manager init failed", "error", err)
		return exitIdentity
	}
	ident, err := idMgr.LoadOrCreate()
	if err != nil {
		logger.Error("identity load failed", "error", err)
		return exitIdentity
	}
	if err := idMgr.Validate(ident); err != nil {
		logger.Error("identity invalid", "error", err)
		return exitIdentity
	}
	logger.Info("Identity loaded", "agent_id", ident.AgentID, "install_id", ident.InstallID)
	logger.Info("Service starting")

	modules := buildModules(cfg, idMgr, log.Logger, logger)

	application, err := app.New(cfg.General.ShutdownTimeout, log.Logger, modules...)
	if err != nil {
		logger.Error("application init failed", "error", err)
		return exitApp
	}
	application.UseConfiguration(cfgMgr)
	application.UseIdentity(idMgr)

	handler := &service.Handler{
		App:     application,
		Logger:  log.Logger.With("component", "service"),
		Timeout: cfg.General.ShutdownTimeout,
	}
	for _, module := range modules {
		if sink, ok := module.(service.SessionChangeSink); ok {
			handler.SessionChange = sink
		}
	}

	if err := service.Run(cfg.General.ServiceName, handler, useConsole); err != nil {
		logger.Error("service failed", "error", err)
		return exitService
	}

	switch handler.ExitCode() {
	case 0:
		return exitOK
	case 50:
		return exitShutdown
	case 40:
		return exitApp
	default:
		return exitService
	}
}

// buildModules assembles the module list from the current configuration.
// A misconfigured or disabled module is logged and skipped rather than
// aborting service startup.
func buildModules(cfg model.Config, idMgr *idmanager.Manager, base, logger *slog.Logger) []interfaces.Module {
	var modules []interfaces.Module

	dev, err := device.New(device.Config{
		Enabled:          true,
		SkipStartCollect: cfg.DeviceUpload.Enabled,
	}, version.Version, base)
	if err != nil {
		logger.Error("device module disabled", "error", err)
	} else {
		modules = append(modules, dev)
	}

	reg, err := registration.New(
		registration.Config{
			Enabled:    cfg.Registration.Enabled,
			BackendURL: cfg.Networking.BackendURL,
			Timeout:    cfg.Registration.Timeout,
		},
		idMgr,
		version.Version,
		base,
	)
	if err != nil {
		logger.Error("registration module disabled", "error", err)
	} else {
		modules = append(modules, reg)
	}

	if dev != nil {
		upload, err := deviceupload.New(
			deviceupload.Config{
				Enabled:    cfg.DeviceUpload.Enabled,
				BackendURL: cfg.Networking.BackendURL,
				Timeout:    cfg.DeviceUpload.Timeout,
				Interval:   cfg.DeviceUpload.Interval,
			},
			idMgr,
			dev,
			version.Version,
			base,
		)
		if err != nil {
			logger.Error("device upload module disabled", "error", err)
		} else {
			modules = append(modules, upload)
		}
	}

	hb, err := heartbeat.New(
		heartbeat.Config{
			Enabled:    cfg.Heartbeat.Enabled,
			BackendURL: cfg.Networking.BackendURL,
			Timeout:    cfg.Heartbeat.Timeout,
			Interval:   cfg.Heartbeat.Interval,
		},
		idMgr,
		version.Version,
		base,
	)
	if err != nil {
		logger.Error("heartbeat module disabled", "error", err)
	} else {
		modules = append(modules, hb)
	}

	usr, err := users.New(
		users.Config{
			Enabled:    cfg.Users.Enabled,
			BackendURL: cfg.Networking.BackendURL,
			Timeout:    cfg.Users.Timeout,
			Interval:   cfg.Users.Interval,
		},
		idMgr,
		version.Version,
		base,
	)
	if err != nil {
		logger.Error("users module disabled", "error", err)
	} else {
		modules = append(modules, usr)
	}

	sw, err := software.New(
		software.Config{
			Enabled:    cfg.Software.Enabled,
			BackendURL: cfg.Networking.BackendURL,
			Timeout:    cfg.Software.Timeout,
			Interval:   cfg.Software.Interval,
		},
		idMgr,
		version.Version,
		base,
	)
	if err != nil {
		logger.Error("software module disabled", "error", err)
	} else {
		modules = append(modules, sw)
	}

	act, err := activity.New(
		activity.Config{
			Enabled:    cfg.Activity.Enabled,
			BackendURL: cfg.Networking.BackendURL,
			Timeout:    cfg.Activity.Timeout,
			Interval:   cfg.Activity.Interval,
			StateDir:   cfg.Storage.StateDir,
		},
		idMgr,
		version.Version,
		base,
	)
	if err != nil {
		logger.Error("activity module disabled", "error", err)
	} else {
		modules = append(modules, act)
	}

	br, err := browseractivity.New(
		browseractivity.Config{
			Enabled:             cfg.BrowserActivity.Enabled,
			BackendURL:          cfg.Networking.BackendURL,
			Timeout:             cfg.BrowserActivity.Timeout,
			Interval:            cfg.BrowserActivity.Interval,
			StateDir:            cfg.Storage.StateDir,
			ChromiumExtensionID: cfg.BrowserActivity.ChromiumExtensionID,
			FirefoxExtensionID:  cfg.BrowserActivity.FirefoxExtensionID,
		},
		idMgr,
		version.Version,
		base,
	)
	if err != nil {
		logger.Error("browser activity module disabled", "error", err)
	} else {
		modules = append(modules, br)
	}

	return modules
}
