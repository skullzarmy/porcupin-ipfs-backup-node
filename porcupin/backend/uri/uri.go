// Package uri provides shared IPFS URI utilities used across backend packages.
package uri

import "strings"

// IsIPFS checks if a string is an IPFS URI (ipfs:// scheme or /ipfs/ gateway path).
func IsIPFS(s string) bool {
	return strings.HasPrefix(s, "ipfs://") || strings.Contains(s, "/ipfs/")
}
