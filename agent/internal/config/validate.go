package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	level := strings.ToLower(strings.TrimSpace(cfg.Agent.LogLevel))
	switch level {
	case "debug", "info", "warn", "error":
		cfg.Agent.LogLevel = level
	default:
		return fmt.Errorf("log_level must be debug, info, warn, or error")
	}

	raw := strings.TrimSpace(cfg.Agent.ShutdownTimeoutRaw)
	if raw == "" {
		return fmt.Errorf("shutdown_timeout is required")
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("shutdown_timeout is invalid: %w", err)
	}
	if timeout <= 0 || timeout > 2*time.Minute {
		return fmt.Errorf("shutdown_timeout must be greater than 0 and at most 2m")
	}
	cfg.Agent.ShutdownTimeout = timeout

	logFile := filepath.Clean(strings.TrimSpace(cfg.Agent.LogFile))
	if logFile == "" || logFile == "." {
		return fmt.Errorf("log_file is required")
	}
	if !filepath.IsAbs(logFile) {
		return fmt.Errorf("log_file must be an absolute path")
	}
	parent := filepath.Dir(logFile)
	if parent == "." || parent == string(filepath.Separator) {
		return fmt.Errorf("log_file parent path is invalid")
	}
	cfg.Agent.LogFile = logFile

	name := strings.TrimSpace(cfg.Service.Name)
	if !validServiceName(name) {
		return fmt.Errorf("service.name is required and must not contain / \\ or control characters")
	}
	cfg.Service.Name = name

	display := strings.TrimSpace(cfg.Service.DisplayName)
	if display == "" {
		display = name
	}
	cfg.Service.DisplayName = display

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
