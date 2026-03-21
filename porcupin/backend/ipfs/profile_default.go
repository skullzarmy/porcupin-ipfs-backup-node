//go:build !lowpower

package ipfs

import (
	"log/slog"
	"time"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core/node/libp2p"
)

// getRoutingOption returns the routing option for the IPFS node
// We universally use DHT Client option to reduce resource usage and avoid being a public server
func getRoutingOption() libp2p.RoutingOption {
	slog.Info("IPFS Profile: using DHT Client routing (Selfish Mode)")
	return libp2p.DHTClientOption
}

// applyProfileConfig applies profile-specific configuration overrides
func applyProfileConfig(cfg *config.Config) {
	slog.Info("IPFS Profile: tuning connection limits for personal usage")

	// Strict connection limits for all users
	// Default is often 600/900 which is excessive for a personal backup tool
	lowWater := config.NewOptionalInteger(20)
	cfg.Swarm.ConnMgr.LowWater = lowWater

	highWater := config.NewOptionalInteger(40)
	cfg.Swarm.ConnMgr.HighWater = highWater

	cfg.Swarm.ConnMgr.GracePeriod = config.NewOptionalDuration(1 * time.Minute)

	// Disable AutoNATService as we are a client
	cfg.AutoNAT.ServiceMode = config.AutoNATServiceDisabled

	// Reduce reprovider interval
	cfg.Reprovider.Interval = config.NewOptionalDuration(12 * time.Hour)
}
