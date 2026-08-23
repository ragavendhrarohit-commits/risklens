//go:build windows

package health

import "testing"

func TestCollectWindowsLiveMetrics(t *testing.T) {
	snap := Collect()
	if snap.LogicalCPUs <= 0 {
		t.Fatalf("logical cpus = %d", snap.LogicalCPUs)
	}
	if snap.PhysicalCores <= 0 {
		t.Fatalf("physical cores = %d", snap.PhysicalCores)
	}
	if snap.Memory.Total == 0 {
		t.Fatal("memory total is zero")
	}
	if snap.Memory.Used+snap.Memory.Free != snap.Memory.Total && snap.Memory.Used > snap.Memory.Total {
		t.Fatalf("memory inconsistent: %+v", snap.Memory)
	}
	if len(snap.Disks) == 0 {
		t.Fatal("expected at least one fixed disk")
	}
	for _, disk := range snap.Disks {
		if disk.Name == "" || disk.Total == 0 {
			t.Fatalf("invalid disk: %+v", disk)
		}
	}
	t.Logf("cpu=%.1f%% model=%q logical=%d physical=%d mem=%.1f%% disks=%d",
		snap.CPUPercent, snap.CPUModel, snap.LogicalCPUs, snap.PhysicalCores, snap.Memory.Percent, len(snap.Disks))
}
