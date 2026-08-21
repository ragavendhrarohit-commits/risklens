package activity

import "context"

// Collector reads the interactive Windows session and foreground window.
// Implementations must never dial the network.
type Collector interface {
	Collect(ctx context.Context) (Observation, error)
}
