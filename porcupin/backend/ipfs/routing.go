package ipfs

import (
	"log/slog"
	"net/url"
	"strings"

	"github.com/ipfs/kubo/config"
)

// autoRouter is the AutoConf placeholder that Kubo expands to the managed IPNI
// indexer endpoint (cid.contact).
const autoRouter = config.AutoPlaceholder // "auto"

// SanitizeDelegatedRouters validates a list of delegated-router entries and
// returns the accepted entries (order-preserving, de-duplicated) alongside any
// rejected entries.
//
// An entry is accepted if it is the literal "auto" (AutoConf/IPNI) or an
// absolute http(s) URL with a host. Blank entries are skipped silently;
// anything else is rejected so a typo can never disable provider discovery
// without a trace in the logs.
func SanitizeDelegatedRouters(raw []string) (accepted []string, rejected []string) {
	seen := make(map[string]struct{}, len(raw))
	for _, entry := range raw {
		e := strings.TrimSpace(entry)
		if e == "" {
			continue
		}
		if _, dup := seen[e]; dup {
			continue
		}
		if isValidRouterEntry(e) {
			seen[e] = struct{}{}
			accepted = append(accepted, e)
		} else {
			rejected = append(rejected, e)
		}
	}
	return accepted, rejected
}

// isValidRouterEntry reports whether an entry is "auto" or an absolute http(s)
// URL with a host component.
func isValidRouterEntry(entry string) bool {
	if entry == autoRouter {
		return true
	}
	u, err := url.Parse(entry)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

// applyDelegatedRouters resolves the effective delegated-router list from the
// caller-supplied configuration and writes it onto the Kubo repo config. It is
// the single authority for provider-routing behaviour and is applied on every
// node start, so the outcome is deterministic regardless of what an older
// version may have persisted in the on-disk repo config.
//
// Semantics:
//   - configured == nil (not set by the caller): default to ["auto"] so IPNI
//     provider discovery is always available. This is the backward-compatible
//     path for repos created before delegated routing existed.
//   - configured is non-nil: use the sanitized entries. If every supplied entry
//     was invalid, fail safe back to ["auto"]. If the caller intentionally
//     supplied an empty list, honour it (DHT-only) but log it clearly.
func applyDelegatedRouters(cfg *config.Config, configured []string) {
	if cfg == nil {
		return
	}

	accepted, rejected := SanitizeDelegatedRouters(configured)
	for _, r := range rejected {
		slog.Warn("ignoring invalid delegated router entry", "value", r)
	}

	switch {
	case configured == nil:
		// Not explicitly configured — guarantee IPNI discovery is enabled.
		accepted = []string{autoRouter}
	case len(accepted) == 0 && len(configured) > 0:
		// Everything supplied was invalid — never leave the node with no
		// provider routing as a result of a typo.
		slog.Warn("no valid delegated routers configured; falling back to IPNI default (cid.contact)")
		accepted = []string{autoRouter}
	case len(accepted) == 0:
		// Caller intentionally provided an empty list.
		slog.Warn("delegated (IPNI) provider routing disabled by config; relying on the DHT only")
	}

	cfg.Routing.DelegatedRouters = accepted

	if len(accepted) > 0 {
		slog.Info("IPFS routing: delegated provider lookups enabled", "routers", accepted)
	}
}
