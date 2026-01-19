//go:build lowpower

package ipfs

import (
	"log"
	"time"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core/node/libp2p"
)

// getRoutingOption returns the routing option for the IPFS node
// For lowpower builds, we use the DHT Client option to reduce resource usage
func getRoutingOption() libp2p.RoutingOption {
	log.Println("Applying LOW POWER profile: Using DHT Client routing")
	return libp2p.DHTClientOption
}

// applyProfileConfig applies profile-specific configuration overrides
// For lowpower builds, we set strict connection limits and GC settings
func applyProfileConfig(cfg *config.Config) {
	log.Println("Applying LOW POWER profile: Tuning connection limits")

	// Strict connection limits for low memory environments (e.g. Raspberry Pi)
	// Default is often 600/900 which is too high for 1GB/2GB RAM if using Kubo default
	lowWater := config.NewOptionalInteger(20)
	cfg.Swarm.ConnMgr.LowWater = lowWater
	
	highWater := config.NewOptionalInteger(40)
	cfg.Swarm.ConnMgr.HighWater = highWater
	
	cfg.Swarm.ConnMgr.GracePeriod = config.NewOptionalDuration(1 * time.Minute)

	// Disable AutoNATService as we are a client
	cfg.AutoNAT.ServiceMode = config.AutoNATServiceDisabled

	// Reduce reprovider interval since we aren't a major content provider
	cfg.Reprovider.Interval = config.NewOptionalDuration(12 * time.Hour)
}
