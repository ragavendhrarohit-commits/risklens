//go:build !windows

package users

import (
	"context"
	"fmt"
	"runtime"
)

// WindowsCollector is the portable fallback used outside Windows.
type WindowsCollector struct{}

// Collect returns an empty inventory with an explanatory error.
func (c *WindowsCollector) Collect(ctx context.Context) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("windows user enumeration unavailable on %s", runtime.GOOS)
}
