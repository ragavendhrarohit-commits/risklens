package generator

import (
	"crypto/rand"
	"fmt"
	"os"
	"runtime"
	"time"

	"risklens/agent/internal/identity/model"
)

type Info struct {
	AgentVersion    string
	DisplayName     string
	MachineName     string
	OperatingSystem string
}

func CollectInfo(agentVersion, displayName string) Info {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	if displayName == "" {
		displayName = host
	}
	return Info{
		AgentVersion:    agentVersion,
		DisplayName:     displayName,
		MachineName:     host,
		OperatingSystem: runtime.GOOS + "/" + runtime.GOARCH,
	}
}

func NewUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func NewIdentity(info Info) (model.Identity, error) {
	agentID, err := NewUUIDv4()
	if err != nil {
		return model.Identity{}, err
	}
	installID, err := NewUUIDv4()
	if err != nil {
		return model.Identity{}, err
	}
	if agentID == installID {
		return model.Identity{}, fmt.Errorf("generated identical agent and install ids")
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	if info.MachineName == "" {
		info.MachineName = "unknown"
	}
	if info.DisplayName == "" {
		info.DisplayName = info.MachineName
	}
	if info.OperatingSystem == "" {
		info.OperatingSystem = runtime.GOOS + "/" + runtime.GOARCH
	}
	if info.AgentVersion == "" {
		info.AgentVersion = "unknown"
	}

	return model.Identity{
		SchemaVersion:   model.SchemaVersion,
		AgentID:         agentID,
		InstallID:       installID,
		AgentVersion:    info.AgentVersion,
		CreatedAt:       now,
		UpdatedAt:       now,
		Status:          model.StatusActive,
		DisplayName:     info.DisplayName,
		MachineName:     info.MachineName,
		OperatingSystem: info.OperatingSystem,
	}, nil
}

func ApplyInfo(id model.Identity, info Info) model.Identity {
	now := time.Now().UTC().Truncate(time.Millisecond)
	changed := false
	if info.AgentVersion != "" && id.AgentVersion != info.AgentVersion {
		id.AgentVersion = info.AgentVersion
		changed = true
	}
	if info.DisplayName != "" && id.DisplayName != info.DisplayName {
		id.DisplayName = info.DisplayName
		changed = true
	}
	if info.MachineName != "" && id.MachineName != info.MachineName {
		id.MachineName = info.MachineName
		changed = true
	}
	if info.OperatingSystem != "" && id.OperatingSystem != info.OperatingSystem {
		id.OperatingSystem = info.OperatingSystem
		changed = true
	}
	if changed {
		id.UpdatedAt = now
	}
	return id
}
