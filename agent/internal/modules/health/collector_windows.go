//go:build windows

package health

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	driveFixed            = 3
	relationProcessorCore = 0
	cpuSampleInterval     = 120 * time.Millisecond
)

var (
	modkernel32                          = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes                   = modkernel32.NewProc("GetSystemTimes")
	procGetLogicalProcessorInformation   = modkernel32.NewProc("GetLogicalProcessorInformation")
	procGlobalMemoryStatusEx             = modkernel32.NewProc("GlobalMemoryStatusEx")

	cpuMu       sync.Mutex
	lastIdle    uint64
	lastKernel  uint64
	lastUser    uint64
	haveCPUBase bool
)

func collect() Snapshot {
	snap := Snapshot{
		LogicalCPUs: runtime.NumCPU(),
		Disks:       []DiskStats{},
	}
	snap.CPUModel = readCPUModel()
	snap.PhysicalCores = readPhysicalCores()
	if snap.PhysicalCores <= 0 {
		snap.PhysicalCores = snap.LogicalCPUs
	}
	snap.CPUPercent = readCPUPercent()
	snap.Memory = readMemory()
	snap.Disks = readDisks()
	return snap
}

func readCPUModel() string {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`HARDWARE\DESCRIPTION\System\CentralProcessor\0`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return ""
	}
	defer key.Close()
	model, _, _ := key.GetStringValue("ProcessorNameString")
	return strings.TrimSpace(model)
}

func readPhysicalCores() int {
	var size uint32
	r0, _, _ := procGetLogicalProcessorInformation.Call(0, uintptr(unsafe.Pointer(&size)))
	if r0 != 0 || size == 0 {
		return 0
	}
	buf := make([]byte, size)
	r0, _, err := procGetLogicalProcessorInformation.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r0 == 0 {
		_ = err
		return 0
	}
	const entrySize = int(unsafe.Sizeof(systemLogicalProcessorInformation{}))
	if entrySize == 0 || int(size) < entrySize {
		return 0
	}
	cores := 0
	for offset := 0; offset+entrySize <= int(size); offset += entrySize {
		relationship := *(*uint32)(unsafe.Pointer(&buf[offset+int(unsafe.Offsetof(systemLogicalProcessorInformation{}.Relationship))]))
		if relationship == relationProcessorCore {
			cores++
		}
	}
	return cores
}

type systemLogicalProcessorInformation struct {
	ProcessorMask uintptr
	Relationship  uint32
	_             [4]byte
	Reserved      [2]uint64
}

func readCPUPercent() float64 {
	idle, kernel, user, ok := systemTimes()
	if !ok {
		return 0
	}

	cpuMu.Lock()
	if !haveCPUBase {
		lastIdle, lastKernel, lastUser = idle, kernel, user
		haveCPUBase = true
		cpuMu.Unlock()
		time.Sleep(cpuSampleInterval)
		idle, kernel, user, ok = systemTimes()
		if !ok {
			return 0
		}
		cpuMu.Lock()
	}
	defer cpuMu.Unlock()

	idleDelta := idle - lastIdle
	kernelDelta := kernel - lastKernel
	userDelta := user - lastUser
	lastIdle, lastKernel, lastUser = idle, kernel, user

	total := kernelDelta + userDelta
	if total == 0 {
		return 0
	}
	busy := total - idleDelta
	if busy < 0 {
		busy = 0
	}
	pct := (float64(busy) / float64(total)) * 100
	return round1(clampPercent(pct))
}

func systemTimes() (idle, kernel, user uint64, ok bool) {
	var idleFT, kernelFT, userFT windows.Filetime
	r0, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleFT)),
		uintptr(unsafe.Pointer(&kernelFT)),
		uintptr(unsafe.Pointer(&userFT)),
	)
	if r0 == 0 {
		return 0, 0, 0, false
	}
	return filetimeToUint64(idleFT), filetimeToUint64(kernelFT), filetimeToUint64(userFT), true
}

func filetimeToUint64(ft windows.Filetime) uint64 {
	return (uint64(ft.HighDateTime) << 32) | uint64(ft.LowDateTime)
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

func readMemory() MemoryStats {
	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 || status.TotalPhys == 0 {
		return MemoryStats{}
	}
	free := status.AvailPhys
	if free > status.TotalPhys {
		free = status.TotalPhys
	}
	used := status.TotalPhys - free
	percent := float64(status.MemoryLoad)
	if percent <= 0 && status.TotalPhys > 0 {
		percent = (float64(used) / float64(status.TotalPhys)) * 100
	}
	return MemoryStats{
		Total:   status.TotalPhys,
		Used:    used,
		Free:    free,
		Percent: round1(clampPercent(percent)),
	}
}

func readDisks() []DiskStats {
	buffer := make([]uint16, 512)
	n, err := windows.GetLogicalDriveStrings(uint32(len(buffer)-1), &buffer[0])
	if err != nil || n == 0 {
		return fallbackSystemDisk()
	}
	drives := utf16DriveList(buffer[:n])
	out := make([]DiskStats, 0, len(drives))
	for _, root := range drives {
		ptr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		if windows.GetDriveType(ptr) != driveFixed {
			continue
		}
		total, free, err := diskSpace(root)
		if err != nil || total == 0 {
			continue
		}
		if free > total {
			free = total
		}
		used := total - free
		percent := (float64(used) / float64(total)) * 100
		out = append(out, DiskStats{
			Name:    normalizeDriveName(root),
			Total:   total,
			Used:    used,
			Free:    free,
			Percent: round1(clampPercent(percent)),
		})
	}
	if len(out) == 0 {
		return fallbackSystemDisk()
	}
	return out
}

func fallbackSystemDisk() []DiskStats {
	root := systemDriveRoot()
	total, free, err := diskSpace(root)
	if err != nil || total == 0 {
		return []DiskStats{}
	}
	if free > total {
		free = total
	}
	used := total - free
	return []DiskStats{{
		Name:    normalizeDriveName(root),
		Total:   total,
		Used:    used,
		Free:    free,
		Percent: round1(clampPercent((float64(used) / float64(total)) * 100)),
	}}
}

func systemDriveRoot() string {
	path, err := windows.GetSystemDirectory()
	if err == nil && len(path) >= 2 && path[1] == ':' {
		return strings.ToUpper(path[:2]) + `\`
	}
	if drive := strings.TrimSpace(os.Getenv("SystemDrive")); drive != "" {
		if !strings.HasSuffix(drive, `\`) {
			drive += `\`
		}
		return drive
	}
	return `C:\`
}

func diskSpace(root string) (total, free uint64, err error) {
	ptr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceEx(%s): %w", root, err)
	}
	return totalBytes, totalFreeBytes, nil
}

func utf16DriveList(buf []uint16) []string {
	out := make([]string, 0, 4)
	start := 0
	for i, v := range buf {
		if v != 0 {
			continue
		}
		if i > start {
			out = append(out, windows.UTF16ToString(buf[start:i]))
		}
		start = i + 1
	}
	return out
}

func normalizeDriveName(root string) string {
	root = strings.TrimSpace(root)
	root = strings.ReplaceAll(root, `/`, `\`)
	root = strings.TrimRight(root, `\`)
	if len(root) >= 2 && root[1] == ':' {
		return strings.ToUpper(root[:2])
	}
	return root
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
