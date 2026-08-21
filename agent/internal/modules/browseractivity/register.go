package browseractivity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const defaultFirefoxExtensionID = "browser-activity@risklens.local"

type nativeHostManifest struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Path              string   `json:"path"`
	Type              string   `json:"type"`
	AllowedOrigins    []string `json:"allowed_origins,omitempty"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`
}

func chromiumManifest(exePath, extensionID string) ([]byte, error) {
	m := nativeHostManifest{
		Name:        NativeHostName,
		Description: "RiskLens Browser Activity",
		Path:        exePath,
		Type:        "stdio",
	}
	id := strings.TrimSpace(extensionID)
	if id != "" {
		origin := id
		if !strings.HasPrefix(origin, "chrome-extension://") {
			origin = "chrome-extension://" + strings.Trim(id, "/") + "/"
		}
		if !strings.HasSuffix(origin, "/") {
			origin += "/"
		}
		m.AllowedOrigins = []string{origin}
	}
	return json.MarshalIndent(m, "", "  ")
}

func firefoxManifest(exePath, extensionID string) ([]byte, error) {
	id := strings.TrimSpace(extensionID)
	if id == "" {
		id = defaultFirefoxExtensionID
	}
	m := nativeHostManifest{
		Name:              NativeHostName,
		Description:       "RiskLens Browser Activity",
		Path:              exePath,
		Type:              "stdio",
		AllowedExtensions: []string{id},
	}
	return json.MarshalIndent(m, "", "  ")
}

func writeNativeManifests(stateDir, exePath, chromiumID, firefoxID string) (chromiumPath, firefoxPath string, err error) {
	dir := filepath.Join(stateDir, "native")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	chromiumPath = filepath.Join(dir, NativeHostName+".chromium.json")
	firefoxPath = filepath.Join(dir, NativeHostName+".firefox.json")
	chromeJSON, err := chromiumManifest(exePath, chromiumID)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(chromiumPath, chromeJSON, 0o644); err != nil {
		return "", "", err
	}
	ffJSON, err := firefoxManifest(exePath, firefoxID)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(firefoxPath, ffJSON, 0o644); err != nil {
		return "", "", err
	}
	return chromiumPath, firefoxPath, nil
}
