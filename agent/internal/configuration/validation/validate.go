package validation

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"risklens/agent/internal/configuration/model"
)

var (
	ErrInvalid      = errors.New("configuration invalid")
	ErrIncompatible = errors.New("incompatible configuration schema version")
)

func Validate(cfg *model.Config) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is required", ErrInvalid)
	}
	if cfg.SchemaVersion <= 0 {
		return fmt.Errorf("%w: schema_version is required", ErrInvalid)
	}
	if cfg.SchemaVersion > model.SchemaVersion {
		return fmt.Errorf("%w: got %d, supported %d", ErrIncompatible, cfg.SchemaVersion, model.SchemaVersion)
	}

	level := strings.ToLower(strings.TrimSpace(cfg.Logging.Level))
	switch level {
	case "debug", "info", "warn", "error":
		cfg.Logging.Level = level
	default:
		return fmt.Errorf("%w: logging.level must be debug, info, warn, or error", ErrInvalid)
	}

	raw := strings.TrimSpace(cfg.General.ShutdownTimeoutRaw)
	if raw == "" {
		return fmt.Errorf("%w: general.shutdown_timeout is required", ErrInvalid)
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("%w: general.shutdown_timeout is invalid: %v", ErrInvalid, err)
	}
	if timeout <= 0 || timeout > 2*time.Minute {
		return fmt.Errorf("%w: general.shutdown_timeout must be greater than 0 and at most 2m", ErrInvalid)
	}
	cfg.General.ShutdownTimeout = timeout
	cfg.General.ShutdownTimeoutRaw = raw

	logFile := filepath.Clean(strings.TrimSpace(cfg.Logging.File))
	if err := requireAbsFile("logging.file", logFile); err != nil {
		return err
	}
	cfg.Logging.File = logFile

	name := strings.TrimSpace(cfg.General.ServiceName)
	if !validServiceName(name) {
		return fmt.Errorf("%w: general.service_name is required and must not contain / \\ or control characters", ErrInvalid)
	}
	cfg.General.ServiceName = name
	display := strings.TrimSpace(cfg.General.DisplayName)
	if display == "" {
		display = name
	}
	cfg.General.DisplayName = display

	if cfg.Performance.MaxConcurrency < 0 {
		return fmt.Errorf("%w: performance.max_concurrency cannot be negative", ErrInvalid)
	}

	for i, module := range cfg.Modules.Enabled {
		module = strings.TrimSpace(module)
		if module == "" {
			return fmt.Errorf("%w: modules.enabled[%d] is empty", ErrInvalid, i)
		}
		cfg.Modules.Enabled[i] = module
	}

	stateDir := filepath.Clean(strings.TrimSpace(cfg.Storage.StateDir))
	if err := requireAbsDir("storage.state_dir", stateDir); err != nil {
		return err
	}
	cfg.Storage.StateDir = stateDir

	identityFile := filepath.Clean(strings.TrimSpace(cfg.Storage.IdentityFile))
	if err := requireAbsFile("storage.identity_file", identityFile); err != nil {
		return err
	}
	cfg.Storage.IdentityFile = identityFile

	if err := validateRegistration(cfg); err != nil {
		return err
	}
	if err := validateDeviceUpload(cfg); err != nil {
		return err
	}
	if err := validateHeartbeat(cfg); err != nil {
		return err
	}
	if err := validateUsers(cfg); err != nil {
		return err
	}
	if err := validateSoftware(cfg); err != nil {
		return err
	}
	if err := validateActivity(cfg); err != nil {
		return err
	}
	if err := validateBrowserActivity(cfg); err != nil {
		return err
	}
	return nil
}

func validateRegistration(cfg *model.Config) error {
	raw := strings.TrimSpace(cfg.Registration.TimeoutRaw)
	if raw == "" {
		if cfg.Registration.Timeout <= 0 {
			cfg.Registration.Timeout = 15 * time.Second
		}
		cfg.Registration.TimeoutRaw = cfg.Registration.Timeout.String()
	} else {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: registration.timeout invalid: %v", ErrInvalid, err)
		}
		if d <= 0 || d > 5*time.Minute {
			return fmt.Errorf("%w: registration.timeout must be > 0 and <= 5m", ErrInvalid)
		}
		cfg.Registration.Timeout = d
		cfg.Registration.TimeoutRaw = raw
	}

	backendURL := strings.TrimSpace(cfg.Networking.BackendURL)
	cfg.Networking.BackendURL = strings.TrimRight(backendURL, "/")

	if cfg.Registration.Enabled {
		if err := requireBackendURL(cfg.Networking.BackendURL); err != nil {
			return err
		}
	}
	return nil
}

func validateDeviceUpload(cfg *model.Config) error {
	raw := strings.TrimSpace(cfg.DeviceUpload.TimeoutRaw)
	if raw == "" {
		if cfg.DeviceUpload.Timeout <= 0 {
			cfg.DeviceUpload.Timeout = 15 * time.Second
		}
		cfg.DeviceUpload.TimeoutRaw = cfg.DeviceUpload.Timeout.String()
	} else {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: device_upload.timeout invalid: %v", ErrInvalid, err)
		}
		if d <= 0 || d > 5*time.Minute {
			return fmt.Errorf("%w: device_upload.timeout must be > 0 and <= 5m", ErrInvalid)
		}
		cfg.DeviceUpload.Timeout = d
		cfg.DeviceUpload.TimeoutRaw = raw
	}

	intervalRaw := strings.TrimSpace(cfg.DeviceUpload.IntervalRaw)
	if intervalRaw == "" {
		if cfg.DeviceUpload.Interval <= 0 {
			cfg.DeviceUpload.Interval = 5 * time.Minute
		}
		cfg.DeviceUpload.IntervalRaw = cfg.DeviceUpload.Interval.String()
	} else {
		d, err := time.ParseDuration(intervalRaw)
		if err != nil {
			return fmt.Errorf("%w: device_upload.interval invalid: %v", ErrInvalid, err)
		}
		if d < time.Minute || d > time.Hour {
			return fmt.Errorf("%w: device_upload.interval must be >= 1m and <= 1h", ErrInvalid)
		}
		cfg.DeviceUpload.Interval = d
		cfg.DeviceUpload.IntervalRaw = intervalRaw
	}

	backendURL := strings.TrimSpace(cfg.Networking.BackendURL)
	cfg.Networking.BackendURL = strings.TrimRight(backendURL, "/")

	if cfg.DeviceUpload.Enabled {
		if err := requireBackendURL(cfg.Networking.BackendURL); err != nil {
			return err
		}
	}
	return nil
}

func validateHeartbeat(cfg *model.Config) error {
	raw := strings.TrimSpace(cfg.Heartbeat.TimeoutRaw)
	if raw == "" {
		if cfg.Heartbeat.Timeout <= 0 {
			cfg.Heartbeat.Timeout = 10 * time.Second
		}
		cfg.Heartbeat.TimeoutRaw = cfg.Heartbeat.Timeout.String()
	} else {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: heartbeat.timeout invalid: %v", ErrInvalid, err)
		}
		if d <= 0 || d > 5*time.Minute {
			return fmt.Errorf("%w: heartbeat.timeout must be > 0 and <= 5m", ErrInvalid)
		}
		cfg.Heartbeat.Timeout = d
		cfg.Heartbeat.TimeoutRaw = raw
	}

	intervalRaw := strings.TrimSpace(cfg.Heartbeat.IntervalRaw)
	if intervalRaw == "" {
		if cfg.Heartbeat.Interval <= 0 {
			cfg.Heartbeat.Interval = 30 * time.Second
		}
		cfg.Heartbeat.IntervalRaw = cfg.Heartbeat.Interval.String()
	} else {
		d, err := time.ParseDuration(intervalRaw)
		if err != nil {
			return fmt.Errorf("%w: heartbeat.interval invalid: %v", ErrInvalid, err)
		}
		if d < time.Second || d > 10*time.Minute {
			return fmt.Errorf("%w: heartbeat.interval must be >= 1s and <= 10m", ErrInvalid)
		}
		cfg.Heartbeat.Interval = d
		cfg.Heartbeat.IntervalRaw = intervalRaw
	}

	backendURL := strings.TrimSpace(cfg.Networking.BackendURL)
	cfg.Networking.BackendURL = strings.TrimRight(backendURL, "/")

	if cfg.Heartbeat.Enabled {
		if err := requireBackendURL(cfg.Networking.BackendURL); err != nil {
			return err
		}
	}
	return nil
}

func validateUsers(cfg *model.Config) error {
	raw := strings.TrimSpace(cfg.Users.TimeoutRaw)
	if raw == "" {
		if cfg.Users.Timeout <= 0 {
			cfg.Users.Timeout = 30 * time.Second
		}
		cfg.Users.TimeoutRaw = cfg.Users.Timeout.String()
	} else {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: users.timeout invalid: %v", ErrInvalid, err)
		}
		if d <= 0 || d > 5*time.Minute {
			return fmt.Errorf("%w: users.timeout must be > 0 and <= 5m", ErrInvalid)
		}
		cfg.Users.Timeout = d
		cfg.Users.TimeoutRaw = raw
	}

	intervalRaw := strings.TrimSpace(cfg.Users.IntervalRaw)
	if intervalRaw == "" {
		if cfg.Users.Interval <= 0 {
			cfg.Users.Interval = 5 * time.Minute
		}
		cfg.Users.IntervalRaw = cfg.Users.Interval.String()
	} else {
		d, err := time.ParseDuration(intervalRaw)
		if err != nil {
			return fmt.Errorf("%w: users.interval invalid: %v", ErrInvalid, err)
		}
		if d < time.Second || d > time.Hour {
			return fmt.Errorf("%w: users.interval must be >= 1s and <= 1h", ErrInvalid)
		}
		cfg.Users.Interval = d
		cfg.Users.IntervalRaw = intervalRaw
	}

	backendURL := strings.TrimSpace(cfg.Networking.BackendURL)
	cfg.Networking.BackendURL = strings.TrimRight(backendURL, "/")

	if cfg.Users.Enabled {
		if err := requireBackendURL(cfg.Networking.BackendURL); err != nil {
			return err
		}
	}
	return nil
}

func validateSoftware(cfg *model.Config) error {
	raw := strings.TrimSpace(cfg.Software.TimeoutRaw)
	if raw == "" {
		if cfg.Software.Timeout <= 0 {
			cfg.Software.Timeout = 60 * time.Second
		}
		cfg.Software.TimeoutRaw = cfg.Software.Timeout.String()
	} else {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: software.timeout invalid: %v", ErrInvalid, err)
		}
		if d <= 0 || d > 10*time.Minute {
			return fmt.Errorf("%w: software.timeout must be > 0 and <= 10m", ErrInvalid)
		}
		cfg.Software.Timeout = d
		cfg.Software.TimeoutRaw = raw
	}

	intervalRaw := strings.TrimSpace(cfg.Software.IntervalRaw)
	if intervalRaw == "" {
		if cfg.Software.Interval <= 0 {
			cfg.Software.Interval = 15 * time.Minute
		}
		cfg.Software.IntervalRaw = cfg.Software.Interval.String()
	} else {
		d, err := time.ParseDuration(intervalRaw)
		if err != nil {
			return fmt.Errorf("%w: software.interval invalid: %v", ErrInvalid, err)
		}
		if d < time.Second || d > 24*time.Hour {
			return fmt.Errorf("%w: software.interval must be >= 1s and <= 24h", ErrInvalid)
		}
		cfg.Software.Interval = d
		cfg.Software.IntervalRaw = intervalRaw
	}

	backendURL := strings.TrimSpace(cfg.Networking.BackendURL)
	cfg.Networking.BackendURL = strings.TrimRight(backendURL, "/")

	if cfg.Software.Enabled {
		if err := requireBackendURL(cfg.Networking.BackendURL); err != nil {
			return err
		}
	}
	return nil
}

func validateActivity(cfg *model.Config) error {
	raw := strings.TrimSpace(cfg.Activity.TimeoutRaw)
	if raw == "" {
		if cfg.Activity.Timeout <= 0 {
			cfg.Activity.Timeout = 15 * time.Second
		}
		cfg.Activity.TimeoutRaw = cfg.Activity.Timeout.String()
	} else {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: activity.timeout invalid: %v", ErrInvalid, err)
		}
		if d <= 0 || d > 5*time.Minute {
			return fmt.Errorf("%w: activity.timeout must be > 0 and <= 5m", ErrInvalid)
		}
		cfg.Activity.Timeout = d
		cfg.Activity.TimeoutRaw = raw
	}

	intervalRaw := strings.TrimSpace(cfg.Activity.IntervalRaw)
	if intervalRaw == "" {
		if cfg.Activity.Interval <= 0 {
			cfg.Activity.Interval = 15 * time.Second
		}
		cfg.Activity.IntervalRaw = cfg.Activity.Interval.String()
	} else {
		d, err := time.ParseDuration(intervalRaw)
		if err != nil {
			return fmt.Errorf("%w: activity.interval invalid: %v", ErrInvalid, err)
		}
		if d < time.Second || d > time.Hour {
			return fmt.Errorf("%w: activity.interval must be >= 1s and <= 1h", ErrInvalid)
		}
		cfg.Activity.Interval = d
		cfg.Activity.IntervalRaw = intervalRaw
	}

	backendURL := strings.TrimSpace(cfg.Networking.BackendURL)
	cfg.Networking.BackendURL = strings.TrimRight(backendURL, "/")

	if cfg.Activity.Enabled {
		if err := requireBackendURL(cfg.Networking.BackendURL); err != nil {
			return err
		}
	}
	return nil
}

func validateBrowserActivity(cfg *model.Config) error {
	raw := strings.TrimSpace(cfg.BrowserActivity.TimeoutRaw)
	if raw == "" {
		if cfg.BrowserActivity.Timeout <= 0 {
			cfg.BrowserActivity.Timeout = 15 * time.Second
		}
		cfg.BrowserActivity.TimeoutRaw = cfg.BrowserActivity.Timeout.String()
	} else {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: browser_activity.timeout invalid: %v", ErrInvalid, err)
		}
		if d <= 0 || d > 5*time.Minute {
			return fmt.Errorf("%w: browser_activity.timeout must be > 0 and <= 5m", ErrInvalid)
		}
		cfg.BrowserActivity.Timeout = d
		cfg.BrowserActivity.TimeoutRaw = raw
	}

	intervalRaw := strings.TrimSpace(cfg.BrowserActivity.IntervalRaw)
	if intervalRaw == "" {
		if cfg.BrowserActivity.Interval <= 0 {
			cfg.BrowserActivity.Interval = 15 * time.Second
		}
		cfg.BrowserActivity.IntervalRaw = cfg.BrowserActivity.Interval.String()
	} else {
		d, err := time.ParseDuration(intervalRaw)
		if err != nil {
			return fmt.Errorf("%w: browser_activity.interval invalid: %v", ErrInvalid, err)
		}
		if d < 15*time.Second || d > time.Hour {
			return fmt.Errorf("%w: browser_activity.interval must be >= 15s and <= 1h", ErrInvalid)
		}
		cfg.BrowserActivity.Interval = d
		cfg.BrowserActivity.IntervalRaw = intervalRaw
	}

	if strings.TrimSpace(cfg.BrowserActivity.FirefoxExtensionID) == "" {
		cfg.BrowserActivity.FirefoxExtensionID = "browser-activity@risklens.local"
	}

	backendURL := strings.TrimSpace(cfg.Networking.BackendURL)
	cfg.Networking.BackendURL = strings.TrimRight(backendURL, "/")

	if cfg.BrowserActivity.Enabled {
		if err := requireBackendURL(cfg.Networking.BackendURL); err != nil {
			return err
		}
	}
	return nil
}

func requireBackendURL(backendURL string) error {
	if backendURL == "" {
		return fmt.Errorf("%w: networking.backend_url is required when registration, device_upload, heartbeat, users, software, activity, or browser_activity is enabled", ErrInvalid)
	}
	u, err := url.Parse(backendURL)
	if err != nil {
		return fmt.Errorf("%w: networking.backend_url is invalid: %v", ErrInvalid, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: networking.backend_url scheme must be http or https", ErrInvalid)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: networking.backend_url host is required", ErrInvalid)
	}
	return nil
}

func requireAbsFile(field, path string) error {
	if path == "" || path == "." {
		return fmt.Errorf("%w: %s is required", ErrInvalid, field)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: %s must be an absolute path", ErrInvalid, field)
	}
	parent := filepath.Dir(path)
	if parent == "." || parent == string(filepath.Separator) {
		return fmt.Errorf("%w: %s parent path is invalid", ErrInvalid, field)
	}
	return nil
}

func requireAbsDir(field, path string) error {
	if path == "" || path == "." {
		return fmt.Errorf("%w: %s is required", ErrInvalid, field)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: %s must be an absolute path", ErrInvalid, field)
	}
	return nil
}

func validServiceName(name string) bool {
	if name == "" || len(name) > 256 {
		return false
	}
	for _, r := range name {
		if r < 32 || r == 127 || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}
