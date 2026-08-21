//go:build windows

package browseractivity

import (
	"log/slog"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func registerNativeHost(logger *slog.Logger, chromiumJSON, firefoxJSON, chromiumID string) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(chromiumID) != "" {
		setHostKey(logger, `Software\Google\Chrome\NativeMessagingHosts\`+NativeHostName, chromiumJSON)
		setHostKey(logger, `Software\Microsoft\Edge\NativeMessagingHosts\`+NativeHostName, chromiumJSON)
	} else {
		logger.Info("Chrome/Edge native host skipped until browser_activity.chromium_extension_id is set")
	}
	setHostKey(logger, `Software\Mozilla\NativeMessagingHosts\`+NativeHostName, firefoxJSON)
}

func setHostKey(logger *slog.Logger, subkey, manifestPath string) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, subkey, registry.SET_VALUE)
	if err != nil {
		logger.Warn("native messaging registry write failed", "key", subkey, "error", err)
		return
	}
	defer key.Close()
	if err := key.SetStringValue("", manifestPath); err != nil {
		logger.Warn("native messaging registry value failed", "key", subkey, "error", err)
	}
}
