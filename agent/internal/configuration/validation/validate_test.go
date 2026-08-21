package validation

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"risklens/agent/internal/configuration/model"
)

func validCfg(t *testing.T) model.Config {
	t.Helper()
	cfg := model.Defaults()
	cfg.Logging.File = filepath.Join(t.TempDir(), "agent.log")
	cfg.Storage.StateDir = t.TempDir()
	cfg.Storage.IdentityFile = filepath.Join(cfg.Storage.StateDir, "identity.json")
	return cfg
}

func TestValidateOK(t *testing.T) {
	cfg := validCfg(t)
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.General.ShutdownTimeout.Seconds() != 15 {
		t.Fatalf("timeout = %s", cfg.General.ShutdownTimeout)
	}
}

func TestValidateRejectsBadLevel(t *testing.T) {
	cfg := validCfg(t)
	cfg.Logging.Level = "verbose"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRejectsFutureSchema(t *testing.T) {
	cfg := validCfg(t)
	cfg.SchemaVersion = 99
	if err := Validate(&cfg); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRejectsRelativeLogFile(t *testing.T) {
	cfg := validCfg(t)
	cfg.Logging.File = "agent.log"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRejectsBadServiceName(t *testing.T) {
	cfg := validCfg(t)
	cfg.General.ServiceName = `RiskLens\Agent`
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRejectsNegativeConcurrency(t *testing.T) {
	cfg := validCfg(t)
	cfg.Performance.MaxConcurrency = -1
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRegistrationDefaultTimeout(t *testing.T) {
	cfg := validCfg(t)
	cfg.Registration.TimeoutRaw = ""
	cfg.Registration.Timeout = 0
	if err := Validate(&cfg); err != nil {
		t.Fatalf("expected default timeout to be applied, got %v", err)
	}
	if cfg.Registration.Timeout <= 0 {
		t.Fatalf("timeout not defaulted: %s", cfg.Registration.Timeout)
	}
}

func TestValidateRegistrationRejectsBadTimeout(t *testing.T) {
	cfg := validCfg(t)
	cfg.Registration.TimeoutRaw = "not-a-duration"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRegistrationRejectsTooLongTimeout(t *testing.T) {
	cfg := validCfg(t)
	cfg.Registration.TimeoutRaw = "10m"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRegistrationEnabledRequiresBackendURL(t *testing.T) {
	cfg := validCfg(t)
	cfg.Registration.Enabled = true
	cfg.Networking.BackendURL = ""
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRegistrationEnabledRejectsBadScheme(t *testing.T) {
	cfg := validCfg(t)
	cfg.Registration.Enabled = true
	cfg.Networking.BackendURL = "ftp://example.com"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRegistrationEnabledAcceptsValidURL(t *testing.T) {
	cfg := validCfg(t)
	cfg.Registration.Enabled = true
	cfg.Networking.BackendURL = "http://127.0.0.1:8000/"
	if err := Validate(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Networking.BackendURL != "http://127.0.0.1:8000" {
		t.Fatalf("trailing slash not stripped: %q", cfg.Networking.BackendURL)
	}
}

func TestValidateDeviceUploadDefaultInterval(t *testing.T) {
	cfg := validCfg(t)
	cfg.DeviceUpload.IntervalRaw = ""
	cfg.DeviceUpload.Interval = 0
	if err := Validate(&cfg); err != nil {
		t.Fatalf("expected default interval, got %v", err)
	}
	if cfg.DeviceUpload.Interval != 5*time.Minute {
		t.Fatalf("interval = %s", cfg.DeviceUpload.Interval)
	}
}

func TestValidateDeviceUploadAcceptsBounds(t *testing.T) {
	cfg := validCfg(t)
	cfg.DeviceUpload.IntervalRaw = "1m"
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DeviceUpload.Interval != time.Minute {
		t.Fatalf("interval = %s", cfg.DeviceUpload.Interval)
	}
	cfg = validCfg(t)
	cfg.DeviceUpload.IntervalRaw = "1h"
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DeviceUpload.Interval != time.Hour {
		t.Fatalf("interval = %s", cfg.DeviceUpload.Interval)
	}
}

func TestValidateDeviceUploadRejectsShortInterval(t *testing.T) {
	cfg := validCfg(t)
	cfg.DeviceUpload.IntervalRaw = "30s"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateDeviceUploadRejectsLongInterval(t *testing.T) {
	cfg := validCfg(t)
	cfg.DeviceUpload.IntervalRaw = "2h"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateDeviceUploadRejectsBadInterval(t *testing.T) {
	cfg := validCfg(t)
	cfg.DeviceUpload.IntervalRaw = "not-a-duration"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateBrowserActivityRejectsShortInterval(t *testing.T) {
	cfg := validCfg(t)
	cfg.BrowserActivity.IntervalRaw = "1s"
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateBrowserActivityEnabledRequiresBackendURL(t *testing.T) {
	cfg := validCfg(t)
	cfg.BrowserActivity.Enabled = true
	cfg.Networking.BackendURL = ""
	if err := Validate(&cfg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateBrowserActivityAcceptsFifteenSeconds(t *testing.T) {
	cfg := validCfg(t)
	cfg.BrowserActivity.Enabled = true
	cfg.BrowserActivity.IntervalRaw = "15s"
	cfg.Networking.BackendURL = "http://127.0.0.1:8000"
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.BrowserActivity.Interval != 15*time.Second {
		t.Fatalf("interval = %s", cfg.BrowserActivity.Interval)
	}
}
