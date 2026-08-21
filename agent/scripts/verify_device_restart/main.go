package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"risklens/agent/internal/modules/device"
	"risklens/agent/internal/version"
)

func main() {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	collect := func(label string) device.Info {
		mod, err := device.New(device.Config{Enabled: true}, version.Version, logger)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: new: %v\n", label, err)
			os.Exit(1)
		}
		if err := mod.Start(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "%s: start: %v\n", label, err)
			os.Exit(1)
		}
		info := mod.Info()
		fmt.Printf("RUN %s state=%s health=%s machine=%s host=%s os=%q ver=%q build=%s arch=%s cpu=%q cores=%d mem=%d disk=%d free=%d mfr=%q model=%q serial=%q boot=%s user=%q agent=%s\n",
			label, mod.Inventory().State, mod.Health(),
			info.MachineName, info.Hostname, info.OperatingSystem, info.OSVersion, info.WindowsBuild,
			info.Architecture, info.CPUModel, info.CPUCores, info.MemoryBytes, info.DiskSizeBytes, info.DiskFreeBytes,
			info.Manufacturer, info.Model, info.SerialNumber, info.BootTime.UTC().Format(time.RFC3339),
			info.CurrentUser, info.AgentVersion,
		)
		return info
	}

	a := collect("1")
	time.Sleep(2 * time.Second)
	b := collect("2")

	type pair struct{ name, av, bv string }
	checks := []pair{
		{"MachineName", a.MachineName, b.MachineName},
		{"Hostname", a.Hostname, b.Hostname},
		{"OperatingSystem", a.OperatingSystem, b.OperatingSystem},
		{"OSVersion", a.OSVersion, b.OSVersion},
		{"WindowsBuild", a.WindowsBuild, b.WindowsBuild},
		{"Architecture", a.Architecture, b.Architecture},
		{"CPUModel", a.CPUModel, b.CPUModel},
		{"CPUCores", fmt.Sprintf("%d", a.CPUCores), fmt.Sprintf("%d", b.CPUCores)},
		{"MemoryBytes", fmt.Sprintf("%d", a.MemoryBytes), fmt.Sprintf("%d", b.MemoryBytes)},
		{"DiskSizeBytes", fmt.Sprintf("%d", a.DiskSizeBytes), fmt.Sprintf("%d", b.DiskSizeBytes)},
		{"Manufacturer", a.Manufacturer, b.Manufacturer},
		{"Model", a.Model, b.Model},
		{"SerialNumber", a.SerialNumber, b.SerialNumber},
		{"BootTime", a.BootTime.UTC().Format(time.RFC3339), b.BootTime.UTC().Format(time.RFC3339)},
		{"CurrentUser", a.CurrentUser, b.CurrentUser},
		{"AgentVersion", a.AgentVersion, b.AgentVersion},
	}

	ok := true
	fmt.Println("RESTART FIELD COMPARISON")
	for _, c := range checks {
		match := c.av == c.bv
		// Disk free may change slightly between runs.
		if c.name == "DiskFreeBytes" {
			continue
		}
		status := "YES"
		if !match {
			status = "NO"
			ok = false
		}
		fmt.Printf("%-16s match=%s run1=%q run2=%q\n", c.name, status, c.av, c.bv)
	}
	// Disk free: allow small drift
	freeDelta := int64(a.DiskFreeBytes) - int64(b.DiskFreeBytes)
	if freeDelta < 0 {
		freeDelta = -freeDelta
	}
	freeOK := freeDelta < 64*1024*1024 // 64 MiB tolerance for churn
	fmt.Printf("%-16s match=%v delta=%d bytes (tolerance 64MiB)\n", "DiskFreeBytes", freeOK, freeDelta)
	if !freeOK {
		ok = false
	}

	if ok {
		fmt.Println("RESTART_CONSISTENT=YES")
	} else {
		fmt.Println("RESTART_CONSISTENT=NO")
		os.Exit(2)
	}
}
