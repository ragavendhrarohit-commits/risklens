package users

import "context"

// Collector enumerates local Windows user accounts.
// Implementations must never dial the network.
type Collector interface {
	Collect(ctx context.Context) ([]Account, error)
}
