//go:build windows

package device

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// WindowsCollector reads static inventory from Win32 APIs and the registry.
// It never dials the network and never shells out to PowerShell/wmic.
type WindowsCollector struct {
	AgentVersion string
	// SystemDrive overrides the volume used for disk metrics. Empty means
	// derive it from GetSystemDirectory (normally C:\).
	SystemDrive string
	// Now is injectable for deterministic boot-time tests.
	Now func() time.Time
}

// Collect gathers inventory. Individual field failures are recorded and
// collection continues; the returned error is non-nil only when every
// critical probe failed or the context was cancelled.
func (c *WindowsCollector) Collect(ctx context.Context) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}

	now := time.Now
	if c.Now != nil {
		now = c.Now
	}

	info := Info{
		AgentVersion: strings.TrimSpace(c.AgentVersion),
		CollectedAt:  truncateTime(now()),
	}
	var errs []string

	record := func(err error) {
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	hostname, err := os.Hostname()
	record(err)
	info.Hostname = strings.TrimSpace(hostname)
	info.MachineName = firstNonEmpty(os.Getenv("COMPUTERNAME"), info.Hostname)

	info.Architecture = detectArchitecture()

	osName, osVersion, build, err := readOSIdentity()
	record(err)
	info.OperatingSystem = osName
	info.OSVersion = osVersion
	info.WindowsBuild = build

	cpuModel, cores, err := readCPU()
	record(err)
	info.CPUModel = cpuModel
	info.CPUCores = cores

	mem, err := readMemoryBytes()
	record(err)
	info.MemoryBytes = mem

	drive := c.SystemDrive
	if drive == "" {
		drive = systemDrive()
	}
	total, free, err := readDiskBytes(drive)
	record(err)
	info.DiskSizeBytes = total
	info.DiskFreeBytes = free

	manufacturer, model, serial, err := readHardwareIdentity()
	record(err)
	info.Manufacturer = manufacturer
	info.Model = model
	info.SerialNumber = serial

	boot, err := readBootTime(now())
	record(err)
	info.BootTime = boot

	currentUser, err := readCurrentUser()
	record(err)
	info.CurrentUser = currentUser

	if err := ctx.Err(); err != nil {
		return info, err
	}

	if info.IsEmpty() {
		if len(errs) == 0 {
			return info, fmt.Errorf("device inventory empty")
		}
		return info, fmt.Errorf("device inventory empty: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		return info, fmt.Errorf("partial device inventory: %s", strings.Join(errs, "; "))
	}
	return info, nil
}

func detectArchitecture() string {
	if arch := strings.TrimSpace(os.Getenv("PROCESSOR_ARCHITECTURE")); arch != "" {
		switch strings.ToUpper(arch) {
		case "AMD64", "X64":
			return "x86_64"
		case "ARM64":
			return "arm64"
		case "X86":
			return "x86"
		default:
			return strings.ToLower(arch)
		}
	}
	return runtime.GOARCH
}

func readOSIdentity() (name, version, build string, err error) {
	key, openErr := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY,
	)
	if openErr != nil {
		return "", "", "", fmt.Errorf("open CurrentVersion: %w", openErr)
	}
	defer key.Close()

	name, _, _ = key.GetStringValue("ProductName")
	build, _, _ = key.GetStringValue("CurrentBuildNumber")
	if build == "" {
		build, _, _ = key.GetStringValue("CurrentBuild")
	}
	displayVersion, _, _ := key.GetStringValue("DisplayVersion")
	releaseID, _, _ := key.GetStringValue("ReleaseId")
	currentVersion, _, _ := key.GetStringValue("CurrentVersion")

	major, _, majorErr := key.GetIntegerValue("CurrentMajorVersionNumber")
	minor, _, minorErr := key.GetIntegerValue("CurrentMinorVersionNumber")
	switch {
	case majorErr == nil && minorErr == nil:
		version = fmt.Sprintf("%d.%d", major, minor)
	case displayVersion != "":
		version = displayVersion
	case releaseID != "":
		version = releaseID
	default:
		version = currentVersion
	}
	if displayVersion != "" && version != displayVersion {
		// Prefer the human-facing Windows 11 style version (e.g. 23H2) when
		// the numeric major/minor pair is present as well.
		if majorErr == nil {
			version = fmt.Sprintf("%d.%d (%s)", major, minor, displayVersion)
		} else {
			version = displayVersion
		}
	}

	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	build = strings.TrimSpace(build)
	if name == "" && version == "" && build == "" {
		return "", "", "", fmt.Errorf("CurrentVersion registry values missing")
	}
	return name, version, build, nil
}

func readCPU() (model string, cores int, err error) {
	cores = runtime.NumCPU()

	key, openErr := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`HARDWARE\DESCRIPTION\System\CentralProcessor\0`,
		registry.QUERY_VALUE,
	)
	if openErr != nil {
		if cores > 0 {
			return "", cores, fmt.Errorf("open CentralProcessor: %w", openErr)
		}
		return "", 0, fmt.Errorf("open CentralProcessor: %w", openErr)
	}
	defer key.Close()

	model, _, _ = key.GetStringValue("ProcessorNameString")
	model = strings.TrimSpace(model)
	if model == "" && cores == 0 {
		return "", 0, fmt.Errorf("cpu model and core count unavailable")
	}
	return model, cores, nil
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func readMemoryBytes() (uint64, error) {
	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	ret, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx").Call(
		uintptr(unsafe.Pointer(&status)),
	)
	if ret == 0 {
		if callErr != nil {
			return 0, fmt.Errorf("GlobalMemoryStatusEx: %w", callErr)
		}
		return 0, fmt.Errorf("GlobalMemoryStatusEx failed")
	}
	if status.TotalPhys == 0 {
		return 0, fmt.Errorf("GlobalMemoryStatusEx returned zero total memory")
	}
	return status.TotalPhys, nil
}

func systemDrive() string {
	path, err := windows.GetSystemDirectory()
	if err != nil || path == "" {
		if drive := os.Getenv("SystemDrive"); drive != "" {
			return ensureTrailingSlash(drive)
		}
		return `C:\`
	}
	if len(path) >= 2 && path[1] == ':' {
		return strings.ToUpper(path[:2]) + `\`
	}
	return ensureTrailingSlash(path)
}

func ensureTrailingSlash(path string) string {
	if path == "" {
		return `C:\`
	}
	if strings.HasSuffix(path, `\`) || strings.HasSuffix(path, `/`) {
		return path
	}
	if len(path) == 2 && path[1] == ':' {
		return path + `\`
	}
	return path + `\`
}

func readDiskBytes(root string) (total, free uint64, err error) {
	root = ensureTrailingSlash(root)
	ptr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceEx(%s): %w", root, err)
	}
	if totalBytes == 0 {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceEx(%s) returned zero capacity", root)
	}
	return totalBytes, totalFreeBytes, nil
}

func readHardwareIdentity() (manufacturer, model, serial string, err error) {
	key, openErr := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`HARDWARE\DESCRIPTION\System\BIOS`,
		registry.QUERY_VALUE,
	)
	if openErr != nil {
		return "", "", "", fmt.Errorf("open BIOS registry: %w", openErr)
	}
	defer key.Close()

	manufacturer, _, _ = key.GetStringValue("SystemManufacturer")
	model, _, _ = key.GetStringValue("SystemProductName")
	serial, _, _ = key.GetStringValue("SystemSerialNumber")

	manufacturer = sanitizeHardwareField(manufacturer)
	model = sanitizeHardwareField(model)
	serial = sanitizeHardwareField(serial)

	if manufacturer == "" && model == "" && serial == "" {
		return "", "", "", fmt.Errorf("BIOS hardware identity values missing")
	}
	return manufacturer, model, serial, nil
}

func readBootTime(now time.Time) (time.Time, error) {
	ms, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount64").Call()
	uptime := time.Duration(ms) * time.Millisecond
	if uptime <= 0 {
		return time.Time{}, fmt.Errorf("GetTickCount64 returned zero uptime")
	}
	return truncateTime(now.Add(-uptime)), nil
}

func readCurrentUser() (string, error) {
	u, err := user.Current()
	if err == nil && u != nil {
		if name := firstNonEmpty(u.Username, u.Name); name != "" {
			return name, nil
		}
	}

	// Fallback for locked-down service contexts where user.Current fails.
	var size uint32 = 256
	buf := make([]uint16, size)
	if err := windows.GetUserNameEx(windows.NameSamCompatible, &buf[0], &size); err != nil {
		username := strings.TrimSpace(os.Getenv("USERNAME"))
		domain := strings.TrimSpace(os.Getenv("USERDOMAIN"))
		switch {
		case username != "" && domain != "":
			return domain + `\` + username, nil
		case username != "":
			return username, nil
		default:
			return "", fmt.Errorf("current user unavailable: %w", err)
		}
	}
	return strings.TrimSpace(windows.UTF16ToString(buf[:size])), nil
}
