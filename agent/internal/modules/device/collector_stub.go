//go:build !windows

package device

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// WindowsCollector is the portable fallback used when the agent is built
// outside Windows. It fills only the fields available from the Go runtime
// and environment so unit tests can still exercise the module. Real inventory
// requires the Windows build tag.
type WindowsCollector struct {
	AgentVersion string
	SystemDrive  string
	Now          func() time.Time
}

// Collect returns a best-effort inventory from portable APIs only.
func (c *WindowsCollector) Collect(ctx context.Context) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}

	now := time.Now
	if c.Now != nil {
		now = c.Now
	}

	hostname, _ := os.Hostname()
	info := Info{
		Hostname:        strings.TrimSpace(hostname),
		MachineName:     firstNonEmpty(os.Getenv("COMPUTERNAME"), strings.TrimSpace(hostname)),
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
		CPUCores:        runtime.NumCPU(),
		AgentVersion:    strings.TrimSpace(c.AgentVersion),
		CollectedAt:     truncateTime(now()),
	}
	if info.IsEmpty() {
		return info, fmt.Errorf("device inventory unavailable on %s", runtime.GOOS)
	}
	return info, fmt.Errorf("partial device inventory: windows APIs unavailable on %s", runtime.GOOS)
}
