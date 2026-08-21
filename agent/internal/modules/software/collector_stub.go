//go:build !windows

package software

import (
	"context"
	"fmt"
	"runtime"
)

// WindowsCollector is the portable fallback used outside Windows.
type WindowsCollector struct{}

// Collect returns an empty inventory with an explanatory error so tests can
// still exercise the module structure on non-Windows hosts.
func (c *WindowsCollector) Collect(ctx context.Context) ([]Application, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("installed software enumeration unavailable on %s", runtime.GOOS)
}
