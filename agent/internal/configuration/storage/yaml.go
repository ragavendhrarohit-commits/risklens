package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"risklens/agent/internal/configuration/model"
)

const maxConfigBytes = 1 << 20

type document struct {
	SchemaVersion int `yaml:"schema_version,omitempty"`

	Agent   *legacyAgent   `yaml:"agent,omitempty"`
	Service *legacyService `yaml:"service,omitempty"`

	General      *generalDoc      `yaml:"general,omitempty"`
	Logging      *loggingDoc      `yaml:"logging,omitempty"`
	Modules      *modulesDoc      `yaml:"modules,omitempty"`
	Performance  *performanceDoc  `yaml:"performance,omitempty"`
	Storage      *storageDoc      `yaml:"storage,omitempty"`
	Identity     *identityDoc     `yaml:"identity,omitempty"`
	Networking   *networkingDoc   `yaml:"networking,omitempty"`
	Registration *registrationDoc `yaml:"registration,omitempty"`
	DeviceUpload *deviceUploadDoc `yaml:"device_upload,omitempty"`
	Heartbeat    *heartbeatDoc    `yaml:"heartbeat,omitempty"`
	Users        *usersDoc        `yaml:"users,omitempty"`
	Software     *softwareDoc     `yaml:"software,omitempty"`
	Activity     *activityDoc     `yaml:"activity,omitempty"`
	Security     *struct{}        `yaml:"security,omitempty"`
	Update       *struct{}        `yaml:"update,omitempty"`
}

type legacyAgent struct {
	LogLevel        string `yaml:"log_level"`
	LogFile         string `yaml:"log_file"`
	ShutdownTimeout string `yaml:"shutdown_timeout"`
}

type legacyService struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
}

type generalDoc struct {
	ServiceName     string `yaml:"service_name"`
	DisplayName     string `yaml:"display_name"`
	ShutdownTimeout string `yaml:"shutdown_timeout"`
}

type loggingDoc struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

type modulesDoc struct {
	Enabled []string `yaml:"enabled"`
}

type performanceDoc struct {
	MaxConcurrency int `yaml:"max_concurrency"`
}

type storageDoc struct {
	StateDir     string `yaml:"state_dir"`
	IdentityFile string `yaml:"identity_file"`
}

type identityDoc struct {
	DisplayName string `yaml:"display_name"`
}

type networkingDoc struct {
	BackendURL string `yaml:"backend_url"`
}

type registrationDoc struct {
	Enabled *bool  `yaml:"enabled"`
	Timeout string `yaml:"timeout"`
}

type deviceUploadDoc struct {
	Enabled  *bool  `yaml:"enabled"`
	Timeout  string `yaml:"timeout"`
	Interval string `yaml:"interval"`
}

type heartbeatDoc struct {
	Enabled  *bool  `yaml:"enabled"`
	Timeout  string `yaml:"timeout"`
	Interval string `yaml:"interval"`
}

type usersDoc struct {
	Enabled  *bool  `yaml:"enabled"`
	Timeout  string `yaml:"timeout"`
	Interval string `yaml:"interval"`
}

type softwareDoc struct {
	Enabled  *bool  `yaml:"enabled"`
	Timeout  string `yaml:"timeout"`
	Interval string `yaml:"interval"`
}

type activityDoc struct {
	Enabled  *bool  `yaml:"enabled"`
	Timeout  string `yaml:"timeout"`
	Interval string `yaml:"interval"`
}

type YAMLStore struct {
	path string
}

func NewYAMLStore(path string) (*YAMLStore, error) {
	if path == "" {
		return nil, fmt.Errorf("configuration path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("configuration path must be absolute")
	}
	return &YAMLStore{path: filepath.Clean(path)}, nil
}

func (s *YAMLStore) Path() string {
	return s.path
}

func (s *YAMLStore) Load() (model.Config, error) {
	data, err := readFile(s.path)
	if err != nil {
		return model.Config{}, err
	}

	var doc document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return model.Config{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}

	cfg := model.Defaults()
	cfg.Path = s.path
	applyDocument(&cfg, doc)
	return cfg, nil
}

func (s *YAMLStore) Save(cfg model.Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	doc := documentFrom(cfg)
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(s.path)
		if err := os.Rename(tmp, s.path); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	return nil
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("%w: %s: %w", ErrNotFound, path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrNotFound, path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory", ErrCorrupt, path)
	}
	if info.Size() > maxConfigBytes {
		return nil, fmt.Errorf("%w: file exceeds %d bytes", ErrCorrupt, maxConfigBytes)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrCorrupt, path, err)
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("%w: file exceeds %d bytes", ErrCorrupt, maxConfigBytes)
	}
	return data, nil
}

func applyDocument(cfg *model.Config, doc document) {
	if doc.SchemaVersion > 0 {
		cfg.SchemaVersion = doc.SchemaVersion
	}
	if doc.Agent != nil {
		if doc.Agent.LogLevel != "" {
			cfg.Logging.Level = doc.Agent.LogLevel
		}
		if doc.Agent.LogFile != "" {
			cfg.Logging.File = doc.Agent.LogFile
		}
		if doc.Agent.ShutdownTimeout != "" {
			cfg.General.ShutdownTimeoutRaw = doc.Agent.ShutdownTimeout
		}
	}
	if doc.Service != nil {
		if doc.Service.Name != "" {
			cfg.General.ServiceName = doc.Service.Name
		}
		if doc.Service.DisplayName != "" {
			cfg.General.DisplayName = doc.Service.DisplayName
		}
	}
	if doc.General != nil {
		if doc.General.ServiceName != "" {
			cfg.General.ServiceName = doc.General.ServiceName
		}
		if doc.General.DisplayName != "" {
			cfg.General.DisplayName = doc.General.DisplayName
		}
		if doc.General.ShutdownTimeout != "" {
			cfg.General.ShutdownTimeoutRaw = doc.General.ShutdownTimeout
		}
	}
	if doc.Logging != nil {
		if doc.Logging.Level != "" {
			cfg.Logging.Level = doc.Logging.Level
		}
		if doc.Logging.File != "" {
			cfg.Logging.File = doc.Logging.File
		}
	}
	if doc.Modules != nil {
		cfg.Modules.Enabled = append([]string(nil), doc.Modules.Enabled...)
	}
	if doc.Performance != nil {
		cfg.Performance.MaxConcurrency = doc.Performance.MaxConcurrency
	}
	if doc.Storage != nil {
		if doc.Storage.StateDir != "" {
			cfg.Storage.StateDir = doc.Storage.StateDir
		}
		if doc.Storage.IdentityFile != "" {
			cfg.Storage.IdentityFile = doc.Storage.IdentityFile
		}
	}
	if doc.Identity != nil && doc.Identity.DisplayName != "" {
		cfg.Identity.DisplayName = doc.Identity.DisplayName
	}
	if doc.Networking != nil && doc.Networking.BackendURL != "" {
		cfg.Networking.BackendURL = doc.Networking.BackendURL
	}
	if doc.Registration != nil {
		if doc.Registration.Enabled != nil {
			cfg.Registration.Enabled = *doc.Registration.Enabled
		}
		if doc.Registration.Timeout != "" {
			cfg.Registration.TimeoutRaw = doc.Registration.Timeout
		}
	}
	if doc.DeviceUpload != nil {
		if doc.DeviceUpload.Enabled != nil {
			cfg.DeviceUpload.Enabled = *doc.DeviceUpload.Enabled
		}
		if doc.DeviceUpload.Timeout != "" {
			cfg.DeviceUpload.TimeoutRaw = doc.DeviceUpload.Timeout
		}
		if doc.DeviceUpload.Interval != "" {
			cfg.DeviceUpload.IntervalRaw = doc.DeviceUpload.Interval
		}
	}
	if doc.Heartbeat != nil {
		if doc.Heartbeat.Enabled != nil {
			cfg.Heartbeat.Enabled = *doc.Heartbeat.Enabled
		}
		if doc.Heartbeat.Timeout != "" {
			cfg.Heartbeat.TimeoutRaw = doc.Heartbeat.Timeout
		}
		if doc.Heartbeat.Interval != "" {
			cfg.Heartbeat.IntervalRaw = doc.Heartbeat.Interval
		}
	}
	if doc.Users != nil {
		if doc.Users.Enabled != nil {
			cfg.Users.Enabled = *doc.Users.Enabled
		}
		if doc.Users.Timeout != "" {
			cfg.Users.TimeoutRaw = doc.Users.Timeout
		}
		if doc.Users.Interval != "" {
			cfg.Users.IntervalRaw = doc.Users.Interval
		}
	}
	if doc.Software != nil {
		if doc.Software.Enabled != nil {
			cfg.Software.Enabled = *doc.Software.Enabled
		}
		if doc.Software.Timeout != "" {
			cfg.Software.TimeoutRaw = doc.Software.Timeout
		}
		if doc.Software.Interval != "" {
			cfg.Software.IntervalRaw = doc.Software.Interval
		}
	}
	if doc.Activity != nil {
		if doc.Activity.Enabled != nil {
			cfg.Activity.Enabled = *doc.Activity.Enabled
		}
		if doc.Activity.Timeout != "" {
			cfg.Activity.TimeoutRaw = doc.Activity.Timeout
		}
		if doc.Activity.Interval != "" {
			cfg.Activity.IntervalRaw = doc.Activity.Interval
		}
	}
}

func documentFrom(cfg model.Config) document {
	return document{
		SchemaVersion: cfg.SchemaVersion,
		Agent: &legacyAgent{
			LogLevel:        cfg.Logging.Level,
			LogFile:         cfg.Logging.File,
			ShutdownTimeout: cfg.General.ShutdownTimeoutRaw,
		},
		Service: &legacyService{
			Name:        cfg.General.ServiceName,
			DisplayName: cfg.General.DisplayName,
		},
		General: &generalDoc{
			ServiceName:     cfg.General.ServiceName,
			DisplayName:     cfg.General.DisplayName,
			ShutdownTimeout: cfg.General.ShutdownTimeoutRaw,
		},
		Logging: &loggingDoc{
			Level: cfg.Logging.Level,
			File:  cfg.Logging.File,
		},
		Modules: &modulesDoc{
			Enabled: cfg.Modules.Enabled,
		},
		Performance: &performanceDoc{
			MaxConcurrency: cfg.Performance.MaxConcurrency,
		},
		Storage: &storageDoc{
			StateDir:     cfg.Storage.StateDir,
			IdentityFile: cfg.Storage.IdentityFile,
		},
		Identity: &identityDoc{
			DisplayName: cfg.Identity.DisplayName,
		},
		Networking: &networkingDoc{
			BackendURL: cfg.Networking.BackendURL,
		},
		Registration: &registrationDoc{
			Enabled: boolPtr(cfg.Registration.Enabled),
			Timeout: cfg.Registration.TimeoutRaw,
		},
		DeviceUpload: &deviceUploadDoc{
			Enabled:  boolPtr(cfg.DeviceUpload.Enabled),
			Timeout:  cfg.DeviceUpload.TimeoutRaw,
			Interval: cfg.DeviceUpload.IntervalRaw,
		},
		Heartbeat: &heartbeatDoc{
			Enabled:  boolPtr(cfg.Heartbeat.Enabled),
			Timeout:  cfg.Heartbeat.TimeoutRaw,
			Interval: cfg.Heartbeat.IntervalRaw,
		},
		Users: &usersDoc{
			Enabled:  boolPtr(cfg.Users.Enabled),
			Timeout:  cfg.Users.TimeoutRaw,
			Interval: cfg.Users.IntervalRaw,
		},
		Software: &softwareDoc{
			Enabled:  boolPtr(cfg.Software.Enabled),
			Timeout:  cfg.Software.TimeoutRaw,
			Interval: cfg.Software.IntervalRaw,
		},
		Activity: &activityDoc{
			Enabled:  boolPtr(cfg.Activity.Enabled),
			Timeout:  cfg.Activity.TimeoutRaw,
			Interval: cfg.Activity.IntervalRaw,
		},
		Security: &struct{}{},
		Update:   &struct{}{},
	}
}

func boolPtr(b bool) *bool { return &b }
