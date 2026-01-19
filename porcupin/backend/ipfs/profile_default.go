//go:build !lowpower

package ipfs

import (
	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core/node/libp2p"
)

// getRoutingOption returns the routing option for the IPFS node
// For standard builds, we use the default DHT option (Server/Hybrid)
func getRoutingOption() libp2p.RoutingOption {
	return libp2p.DHTOption
}

// applyProfileConfig applies profile-specific configuration overrides
// For standard builds, we use the defaults
func applyProfileConfig(cfg *config.Config) {
	// No overrides for standard profile
}
