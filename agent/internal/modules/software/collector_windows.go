//go:build windows

package software

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// WindowsCollector reads installed applications from the standard uninstall
// registry hives. It never dials the network and never shells out to
// PowerShell/wmic.
type WindowsCollector struct{}

// registrySource is one hive+subkey+architecture the collector will walk.
type registrySource struct {
	Key          registry.Key
	Subkey       string
	Architecture string
	Display      string // human-readable prefix used for RegistryPath
}

// systemSources enumerates the machine-wide uninstall keys.
// Per-user keys are appended per current-user session.
func systemSources() []registrySource {
	return []registrySource{
		{
			Key:          registry.LOCAL_MACHINE,
			Subkey:       `Software\Microsoft\Windows\CurrentVersion\Uninstall`,
			Architecture: ArchitectureX64,
			Display:      `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall`,
		},
		{
			Key:          registry.LOCAL_MACHINE,
			Subkey:       `Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
			Architecture: ArchitectureX86,
			Display:      `HKLM\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
		},
	}
}

// userSources enumerates per-user uninstall keys under HKCU.
// This is best-effort: if the service runs as SYSTEM these are empty.
func userSources() []registrySource {
	return []registrySource{
		{
			Key:          registry.CURRENT_USER,
			Subkey:       `Software\Microsoft\Windows\CurrentVersion\Uninstall`,
			Architecture: ArchitectureX64,
			Display:      `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall`,
		},
	}
}

// Collect enumerates installed applications from all uninstall keys.
// Partial results are returned when only some hives were readable.
func (c *WindowsCollector) Collect(ctx context.Context) ([]Application, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var (
		apps       []Application
		partialErr []string
	)

	all := append([]registrySource{}, systemSources()...)
	all = append(all, userSources()...)

	for _, src := range all {
		if err := ctx.Err(); err != nil {
			return apps, err
		}
		entries, err := readSource(src)
		if err != nil {
			// A missing per-user hive is expected; only record real errors.
			if !errors.Is(err, registry.ErrNotExist) {
				partialErr = append(partialErr, fmt.Sprintf("%s: %v", src.Display, err))
			}
			continue
		}
		apps = append(apps, entries...)
	}

	apps = dedupe(apps)

	if len(apps) == 0 && len(partialErr) > 0 {
		return nil, fmt.Errorf("software enumeration failed: %s", strings.Join(partialErr, "; "))
	}
	if len(partialErr) > 0 {
		return apps, fmt.Errorf("partial software enumeration: %s", strings.Join(partialErr, "; "))
	}
	return apps, nil
}

func readSource(src registrySource) ([]Application, error) {
	root, err := registry.OpenKey(src.Key, src.Subkey, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}

	out := make([]Application, 0, len(names))
	for _, name := range names {
		entry, ok := readEntry(root, src, name)
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func readEntry(parent registry.Key, src registrySource, subkey string) (Application, bool) {
	handle, err := registry.OpenKey(parent, subkey, registry.QUERY_VALUE)
	if err != nil {
		return Application{}, false
	}
	defer handle.Close()

	displayName, _, _ := handle.GetStringValue("DisplayName")
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Application{}, false
	}

	version, _, _ := handle.GetStringValue("DisplayVersion")
	publisher, _, _ := handle.GetStringValue("Publisher")
	installDate, _, _ := handle.GetStringValue("InstallDate")
	installLocation, _, _ := handle.GetStringValue("InstallLocation")

	var estimatedSize int64
	if raw, _, err := handle.GetIntegerValue("EstimatedSize"); err == nil {
		// Registry stores EstimatedSize in KB.
		estimatedSize = int64(raw) * 1024
	}

	systemComponent := readBoolFlag(handle, "SystemComponent")
	windowsComponent := readBoolFlag(handle, "WindowsInstaller") || readBoolFlag(handle, "WindowsComponent")

	entry := Application{
		Name:             displayName,
		Version:          version,
		Publisher:        publisher,
		InstallDate:      normalizeInstallDate(installDate),
		InstallLocation:  installLocation,
		EstimatedSize:    estimatedSize,
		Architecture:     src.Architecture,
		SystemComponent:  systemComponent,
		WindowsComponent: windowsComponent,
		RegistryPath:     src.Display + `\` + subkey,
	}
	sanitized, ok := sanitize(entry)
	return sanitized, ok
}

func readBoolFlag(k registry.Key, name string) bool {
	if v, _, err := k.GetIntegerValue(name); err == nil {
		return v != 0
	}
	if v, _, err := k.GetStringValue(name); err == nil {
		return strings.EqualFold(strings.TrimSpace(v), "1")
	}
	return false
}

// normalizeInstallDate accepts the common YYYYMMDD registry format and
// returns YYYY-MM-DD; anything else is returned as-is.
func normalizeInstallDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == 8 {
		digitsOnly := true
		for _, r := range raw {
			if r < '0' || r > '9' {
				digitsOnly = false
				break
			}
		}
		if digitsOnly {
			return raw[0:4] + "-" + raw[4:6] + "-" + raw[6:8]
		}
	}
	return raw
}
