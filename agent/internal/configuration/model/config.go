package model

import (
	"os"
	"path/filepath"
	"time"
)

const SchemaVersion = 1

const (
	EnvConfig                     = "RISKLENS_AGENT_CONFIG"
	EnvLogLevel                   = "RISKLENS_AGENT_LOG_LEVEL"
	EnvLogFile                    = "RISKLENS_AGENT_LOG_FILE"
	EnvShutdownTimeout            = "RISKLENS_AGENT_SHUTDOWN_TIMEOUT"
	EnvBackendURL                 = "RISKLENS_AGENT_BACKEND_URL"
	EnvRegistrationEnabled        = "RISKLENS_AGENT_REGISTRATION_ENABLED"
	EnvRegistrationTimeout        = "RISKLENS_AGENT_REGISTRATION_TIMEOUT"
	EnvDeviceUploadEnabled        = "RISKLENS_AGENT_DEVICE_UPLOAD_ENABLED"
	EnvDeviceUploadTimeout        = "RISKLENS_AGENT_DEVICE_UPLOAD_TIMEOUT"
	EnvDeviceUploadInterval       = "RISKLENS_AGENT_DEVICE_UPLOAD_INTERVAL"
	EnvHeartbeatEnabled           = "RISKLENS_AGENT_HEARTBEAT_ENABLED"
	EnvHeartbeatTimeout           = "RISKLENS_AGENT_HEARTBEAT_TIMEOUT"
	EnvHeartbeatInterval          = "RISKLENS_AGENT_HEARTBEAT_INTERVAL"
	EnvUsersEnabled               = "RISKLENS_AGENT_USERS_ENABLED"
	EnvUsersTimeout               = "RISKLENS_AGENT_USERS_TIMEOUT"
	EnvUsersInterval              = "RISKLENS_AGENT_USERS_INTERVAL"
	EnvSoftwareEnabled            = "RISKLENS_AGENT_SOFTWARE_ENABLED"
	EnvSoftwareTimeout            = "RISKLENS_AGENT_SOFTWARE_TIMEOUT"
	EnvSoftwareInterval           = "RISKLENS_AGENT_SOFTWARE_INTERVAL"
	EnvActivityEnabled            = "RISKLENS_AGENT_ACTIVITY_ENABLED"
	EnvActivityTimeout            = "RISKLENS_AGENT_ACTIVITY_TIMEOUT"
	EnvActivityInterval           = "RISKLENS_AGENT_ACTIVITY_INTERVAL"
	EnvBrowserActivityEnabled     = "RISKLENS_AGENT_BROWSER_ACTIVITY_ENABLED"
	EnvBrowserActivityTimeout     = "RISKLENS_AGENT_BROWSER_ACTIVITY_TIMEOUT"
	EnvBrowserActivityInterval    = "RISKLENS_AGENT_BROWSER_ACTIVITY_INTERVAL"
	EnvBrowserChromiumExtensionID = "RISKLENS_AGENT_BROWSER_CHROMIUM_EXTENSION_ID"
	EnvBrowserFirefoxExtensionID  = "RISKLENS_AGENT_BROWSER_FIREFOX_EXTENSION_ID"
)

// Config is the agent-wide settings snapshot. Security and update sections
// are reserved for later sprints. Networking currently carries only the
// backend URL used by the Registration module.
type Config struct {
	Path            string
	SchemaVersion   int
	General         General
	Logging         Logging
	Modules         Modules
	Performance     Performance
	Storage         Storage
	Identity        Identity
	Networking      Networking
	Registration    Registration
	DeviceUpload    DeviceUpload
	Heartbeat       Heartbeat
	Users           Users
	Software        Software
	Activity        Activity
	BrowserActivity BrowserActivity
	Security        Security
	Update          Update
}

type General struct {
	ServiceName        string
	DisplayName        string
	ShutdownTimeout    time.Duration
	ShutdownTimeoutRaw string
}

type Logging struct {
	Level string
	File  string
}

type Modules struct {
	Enabled []string
}

type Performance struct {
	MaxConcurrency int
}

type Storage struct {
	StateDir     string
	IdentityFile string
}

type Identity struct {
	DisplayName string
}

type Networking struct {
	BackendURL string
}

type Registration struct {
	Enabled    bool
	Timeout    time.Duration
	TimeoutRaw string
}

type DeviceUpload struct {
	Enabled     bool
	Timeout     time.Duration
	TimeoutRaw  string
	Interval    time.Duration
	IntervalRaw string
}

type Heartbeat struct {
	Enabled     bool
	Timeout     time.Duration
	TimeoutRaw  string
	Interval    time.Duration
	IntervalRaw string
}

type Users struct {
	Enabled     bool
	Timeout     time.Duration
	TimeoutRaw  string
	Interval    time.Duration
	IntervalRaw string
}

type Software struct {
	Enabled     bool
	Timeout     time.Duration
	TimeoutRaw  string
	Interval    time.Duration
	IntervalRaw string
}

type Activity struct {
	Enabled     bool
	Timeout     time.Duration
	TimeoutRaw  string
	Interval    time.Duration
	IntervalRaw string
}

type BrowserActivity struct {
	Enabled             bool
	Timeout             time.Duration
	TimeoutRaw          string
	Interval            time.Duration
	IntervalRaw         string
	ChromiumExtensionID string
	FirefoxExtensionID  string
}

type Security struct{}
type Update struct{}

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
	stateDir := filepath.Join(programData(), "RiskLens", "state")
	return Config{
		SchemaVersion: SchemaVersion,
		General: General{
			ServiceName:        "RiskLensAgent",
			DisplayName:        "RiskLens Endpoint Agent",
			ShutdownTimeout:    15 * time.Second,
			ShutdownTimeoutRaw: "15s",
		},
		Logging: Logging{
			Level: "info",
			File:  filepath.Join(programData(), "RiskLens", "logs", "agent.log"),
		},
		Modules:     Modules{},
		Performance: Performance{},
		Storage: Storage{
			StateDir:     stateDir,
			IdentityFile: filepath.Join(stateDir, "identity.json"),
		},
		Identity:   Identity{},
		Networking: Networking{},
		Registration: Registration{
			Enabled:    false,
			Timeout:    15 * time.Second,
			TimeoutRaw: "15s",
		},
		DeviceUpload: DeviceUpload{
			Enabled:     false,
			Timeout:     15 * time.Second,
			TimeoutRaw:  "15s",
			Interval:    5 * time.Minute,
			IntervalRaw: "5m",
		},
		Heartbeat: Heartbeat{
			Enabled:     false,
			Timeout:     10 * time.Second,
			TimeoutRaw:  "10s",
			Interval:    30 * time.Second,
			IntervalRaw: "30s",
		},
		Users: Users{
			Enabled:     false,
			Timeout:     30 * time.Second,
			TimeoutRaw:  "30s",
			Interval:    5 * time.Minute,
			IntervalRaw: "5m",
		},
		Software: Software{
			Enabled:     false,
			Timeout:     60 * time.Second,
			TimeoutRaw:  "60s",
			Interval:    15 * time.Minute,
			IntervalRaw: "15m",
		},
		Activity: Activity{
			Enabled:     false,
			Timeout:     15 * time.Second,
			TimeoutRaw:  "15s",
			Interval:    15 * time.Second,
			IntervalRaw: "15s",
		},
		BrowserActivity: BrowserActivity{
			Enabled:            false,
			Timeout:            15 * time.Second,
			TimeoutRaw:         "15s",
			Interval:           15 * time.Second,
			IntervalRaw:        "15s",
			FirefoxExtensionID: "browser-activity@risklens.local",
		},
		Security: Security{},
		Update:   Update{},
	}
}

func (c Config) Clone() Config {
	out := c
	if c.Modules.Enabled != nil {
		out.Modules.Enabled = append([]string(nil), c.Modules.Enabled...)
	}
	return out
}
