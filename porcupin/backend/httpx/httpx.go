// Package httpx provides shared HTTP client helpers for Porcupin.
//
// Its primary purpose is to ensure every outbound HTTP request Porcupin makes
// carries a descriptive User-Agent header instead of the anonymous default Go
// HTTP client string. Public IPFS gateways and API operators run on a
// best-effort basis and rate-limit or block traffic they cannot identify; a
// clear, professional User-Agent lets them recognise Porcupin, throttle it if
// needed, and find the project/contact details rather than blanket-blocking us.
package httpx

import (
	"net/http"
	"time"

	"porcupin/backend/version"
)

// projectURL is the canonical project home used in the User-Agent, following
// the conventional "+<url>" contact hint format (as used by well-behaved bots).
const projectURL = "https://github.com/skullzarmy/porcupin-ipfs-backup-node"

// userAgent identifies Porcupin on every outbound request. Format:
//
//	Porcupin/<version> (+<project url>)
//
// e.g. "Porcupin/1.0.4 (+https://github.com/skullzarmy/porcupin-ipfs-backup-node)"
var userAgent = "Porcupin/" + version.Version + " (+" + projectURL + ")"

// UserAgent returns the User-Agent string that identifies Porcupin to remote
// servers. Set it as the "User-Agent" header on any request that does not go
// through a client created by this package.
func UserAgent() string {
	return userAgent
}

// userAgentTransport is an http.RoundTripper that injects Porcupin's User-Agent
// on outgoing requests that don't already set one, then delegates to base.
type userAgentTransport struct {
	base http.RoundTripper
}

// RoundTrip implements http.RoundTripper. Per the RoundTripper contract it does
// not mutate the caller's request: when a User-Agent needs to be added the
// request is cloned first.
func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", userAgent)
	}
	return t.base.RoundTrip(req)
}

// Transport wraps base so outgoing requests carry Porcupin's User-Agent. If
// base is nil, http.DefaultTransport is used.
func Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &userAgentTransport{base: base}
}

// NewClient returns an *http.Client with the given timeout that sends
// Porcupin's User-Agent on every request. A zero timeout means no client-level
// timeout (rely on per-request context deadlines).
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: Transport(nil),
	}
}

// Client is a shared *http.Client that injects Porcupin's User-Agent. It has no
// client-level timeout, so callers must use context deadlines for cancellation.
// Use it in place of http.DefaultClient for outbound requests.
var Client = NewClient(0)
