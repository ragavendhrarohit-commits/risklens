//go:build !windows

package health

func collect() Snapshot {
	return Snapshot{Disks: []DiskStats{}}
}
