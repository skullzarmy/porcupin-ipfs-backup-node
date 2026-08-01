//go:build !lowpower

package ipfs

import (
	"log/slog"
	"time"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core/node/libp2p"
)

// getRoutingOption returns the routing option for the IPFS node.
//
// We combine a DHT *client* (no server responsibilities — keeps resource usage
// low, "selfish mode") with Kubo's default HTTP delegated routers (the IPNI
// indexer at cid.contact, resolved via AutoConf). This mirrors Kubo's
// "autoclient" routing.
//
// This matters because most NFT content — Versum, Emprops, and anything stored
// via nft.storage / web3.storage / Filecoin — advertises its provider records
// to the IPNI indexer but NOT to the Amino DHT. A DHT-only client finds zero
// providers for that content, so Bitswap has no peer to fetch from and pins
// time out even though the content is widely available on the network. Adding
// delegated routing lets the node discover those providers.
func getRoutingOption(cfg *config.Config) libp2p.RoutingOption {
	slog.Info("IPFS Profile: using DHT client + delegated IPNI routing")
	return libp2p.ConstructDefaultRouting(cfg, libp2p.DHTClientOption)
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
