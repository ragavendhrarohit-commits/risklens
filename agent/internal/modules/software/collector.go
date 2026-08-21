package software

import (
	"context"
	"sort"
	"strings"
)

// Collector enumerates installed Windows applications.
// Implementations must never dial the network.
type Collector interface {
	Collect(ctx context.Context) ([]Application, error)
}

// sanitize trims and filters blank / obviously invalid entries.
func sanitize(entry Application) (Application, bool) {
	entry.Name = strings.TrimSpace(entry.Name)
	entry.Version = strings.TrimSpace(entry.Version)
	entry.Publisher = strings.TrimSpace(entry.Publisher)
	entry.InstallDate = strings.TrimSpace(entry.InstallDate)
	entry.InstallLocation = strings.TrimSpace(entry.InstallLocation)
	entry.Architecture = strings.TrimSpace(entry.Architecture)
	entry.RegistryPath = strings.TrimSpace(entry.RegistryPath)
	if entry.Name == "" {
		return Application{}, false
	}
	return entry, true
}

// dedupe removes duplicate (name, version) rows while preserving a stable order.
// When a duplicate is seen, the entry with more populated fields wins.
func dedupe(apps []Application) []Application {
	if len(apps) == 0 {
		return apps
	}
	index := make(map[string]int, len(apps))
	out := make([]Application, 0, len(apps))
	for _, app := range apps {
		key := strings.ToLower(app.Name) + "\x00" + strings.ToLower(app.Version)
		if existing, ok := index[key]; ok {
			if score(app) > score(out[existing]) {
				out[existing] = app
			}
			continue
		}
		index[key] = len(out)
		out = append(out, app)
	}
	// Stable alphabetical order for deterministic uploads.
	sort.SliceStable(out, func(i, j int) bool {
		if strings.EqualFold(out[i].Name, out[j].Name) {
			return out[i].Version < out[j].Version
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// score rewards more populated entries so we keep the richer duplicate row.
func score(app Application) int {
	total := 0
	if app.Version != "" {
		total++
	}
	if app.Publisher != "" {
		total++
	}
	if app.InstallDate != "" {
		total++
	}
	if app.InstallLocation != "" {
		total++
	}
	if app.EstimatedSize > 0 {
		total++
	}
	if app.Architecture != "" {
		total++
	}
	return total
}
