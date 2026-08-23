package network

import "context"

// EndpointNetwork is sent with heartbeats for approximate IP-based geolocation.
type EndpointNetwork struct {
	PublicIP  string
	PrivateIP string
	Hostname  string
}

// Collect gathers local network metadata. No GPS or browser location is used.
func Collect(ctx context.Context) EndpointNetwork {
	return collect(ctx)
}
