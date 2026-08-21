//go:build !windows

package activity

import (
	"context"
	"fmt"
	"runtime"
)

// WindowsCollector is the portable fallback used outside Windows.
type WindowsCollector struct{}

// Collect returns an empty observation with an explanatory error.
func (c *WindowsCollector) Collect(ctx context.Context) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	return Observation{}, fmt.Errorf("windows activity collection unavailable on %s", runtime.GOOS)
}

// RunForegroundHelper is unused outside Windows.
func RunForegroundHelper() int {
	return 1
}
