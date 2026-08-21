package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

var (
	ErrMissing = errors.New("config file missing or unreadable")
	ErrInvalid = errors.New("config invalid")
)

const (
	EnvConfig          = "RISKLENS_AGENT_CONFIG"
	EnvLogLevel        = "RISKLENS_AGENT_LOG_LEVEL"
	EnvLogFile         = "RISKLENS_AGENT_LOG_FILE"
	EnvShutdownTimeout = "RISKLENS_AGENT_SHUTDOWN_TIMEOUT"
)

type Config struct {
	Path    string        `yaml:"-"`
	Agent   AgentConfig   `yaml:"agent"`
	Service ServiceConfig `yaml:"service"`
}

type AgentConfig struct {
	LogLevel           string        `yaml:"log_level"`
	LogFile            string        `yaml:"log_file"`
	ShutdownTimeoutRaw string        `yaml:"shutdown_timeout"`
	ShutdownTimeout    time.Duration `yaml:"-"`
}

type ServiceConfig struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
}

// Overrides are applied after the file is loaded. Empty fields are ignored.
type Overrides struct {
	LogLevel        string
	LogFile         string
	ShutdownTimeout string
}

type Result struct {
	Config   Config
	Warnings []string
}

func programData() string {
	base := os.Getenv("PROGRAMDATA")
	if base == "" {
		base = `C:\ProgramData`
	}
	return base
}

func DefaultPath() string {
	return filepath.Join(programData(), "RiskLens", "agent.yaml")
}

func Defaults() Config {
	return Config{
		Agent: AgentConfig{
			LogLevel:           "info",
			LogFile:            filepath.Join(programData(), "RiskLens", "logs", "agent.log"),
			ShutdownTimeoutRaw: "15s",
			ShutdownTimeout:    15 * time.Second,
		},
		Service: ServiceConfig{
			Name:        "RiskLensAgent",
			DisplayName: "RiskLens Endpoint Agent",
		},
	}
}
