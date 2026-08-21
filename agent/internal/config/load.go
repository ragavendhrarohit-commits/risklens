package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const maxConfigBytes = 1 << 20

func Load(path string, flags Overrides) (Result, error) {
	if path == "" {
		path = os.Getenv(EnvConfig)
	}
	if path == "" {
		path = DefaultPath()
	}
	path = filepath.Clean(path)

	data, err := readConfigFile(path)
	if err != nil {
		return Result{}, err
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Result{}, fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}

	cfg := Defaults()
	cfg.Path = path
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Result{}, fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}

	applyOverrides(&cfg, overridesFromEnv())
	applyOverrides(&cfg, flags)

	if err := Validate(&cfg); err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	return Result{
		Config:   cfg,
		Warnings: unknownKeyWarnings(raw),
	}, nil
}

func readConfigFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrMissing, path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrMissing, path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory", ErrInvalid, path)
	}
	if info.Size() > maxConfigBytes {
		return nil, fmt.Errorf("%w: config file exceeds %d bytes", ErrInvalid, maxConfigBytes)
	}

	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrMissing, path, err)
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("%w: config file exceeds %d bytes", ErrInvalid, maxConfigBytes)
	}
	return data, nil
}

func overridesFromEnv() Overrides {
	return Overrides{
		LogLevel:        os.Getenv(EnvLogLevel),
		LogFile:         os.Getenv(EnvLogFile),
		ShutdownTimeout: os.Getenv(EnvShutdownTimeout),
	}
}

func applyOverrides(cfg *Config, o Overrides) {
	if o.LogLevel != "" {
		cfg.Agent.LogLevel = o.LogLevel
	}
	if o.LogFile != "" {
		cfg.Agent.LogFile = o.LogFile
	}
	if o.ShutdownTimeout != "" {
		cfg.Agent.ShutdownTimeoutRaw = o.ShutdownTimeout
	}
}

var knownKeys = map[string][]string{
	"agent":   {"log_level", "log_file", "shutdown_timeout"},
	"service": {"name", "display_name"},
}

func unknownKeyWarnings(raw map[string]any) []string {
	if raw == nil {
		return nil
	}

	var warnings []string
	for key, value := range raw {
		children, ok := knownKeys[key]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("unknown config key %q", key))
			continue
		}
		nested, ok := asStringMap(value)
		if !ok {
			continue
		}
		allowed := make(map[string]struct{}, len(children))
		for _, child := range children {
			allowed[child] = struct{}{}
		}
		for child := range nested {
			if _, ok := allowed[child]; !ok {
				warnings = append(warnings, fmt.Sprintf("unknown config key %q.%q", key, child))
			}
		}
	}
	sort.Strings(warnings)
	return warnings
}

func asStringMap(value any) (map[string]any, bool) {
	if m, ok := value.(map[string]any); ok {
		return m, true
	}
	return nil, false
}
