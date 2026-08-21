//go:build !windows

package browseractivity

import "log/slog"

func registerNativeHost(logger *slog.Logger, chromiumJSON, firefoxJSON, chromiumID string) {
	if logger != nil {
		logger.Info("native messaging registry skipped on this OS")
	}
}
