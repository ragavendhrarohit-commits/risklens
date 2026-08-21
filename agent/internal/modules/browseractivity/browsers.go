package browseractivity

import (
	"path/filepath"
	"strings"
)

const (
	BrowserChrome  = "chrome"
	BrowserEdge    = "edge"
	BrowserFirefox = "firefox"

	ExeChrome  = "chrome.exe"
	ExeEdge    = "msedge.exe"
	ExeFirefox = "firefox.exe"
)

// SupportedBrowsers is the Phase 1 allowlist. Brave/Opera are out of this phase.
var supportedExecutables = map[string]string{
	ExeChrome:  BrowserChrome,
	ExeEdge:    BrowserEdge,
	ExeFirefox: BrowserFirefox,
}

func NormalizeExecutable(value string) string {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(value), "/", `\`))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	return strings.ToLower(name)
}

func IsBrowserExecutable(value string) bool {
	_, ok := supportedExecutables[NormalizeExecutable(value)]
	return ok
}

func ExecutableForBrowser(browser string) string {
	switch strings.ToLower(strings.TrimSpace(browser)) {
	case BrowserEdge:
		return ExeEdge
	case BrowserFirefox:
		return ExeFirefox
	default:
		return ExeChrome
	}
}

func BrowserForExecutable(executable string) string {
	if name, ok := supportedExecutables[NormalizeExecutable(executable)]; ok {
		return name
	}
	return ""
}
