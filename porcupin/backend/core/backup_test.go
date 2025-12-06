package core

import (
	"fmt"
	"testing"

	"porcupin/backend/indexer"
)

func TestExtractCIDFromURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		// Standard ipfs:// URIs
		{
			name:     "simple ipfs:// URI",
			uri:      "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
			expected: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		},
		{
			name:     "ipfs:// with path",
			uri:      "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG/image.png",
			expected: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		},
		{
			name:     "ipfs:// with query params (fxhash style)",
			uri:      "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG?fxhash=oo123",
			expected: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		},
		{
			name:     "ipfs:// with path and query",
			uri:      "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG/index.html?param=value",
			expected: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		},
		// CIDv1 (base32)
		{
			name:     "CIDv1 base32",
			uri:      "ipfs://bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi",
			expected: "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi",
		},
		// Gateway URLs
		{
			name:     "ipfs.io gateway",
			uri:      "https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
			expected: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		},
		{
			name:     "cloudflare gateway",
			uri:      "https://cloudflare-ipfs.com/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
			expected: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		},
		{
			name:     "gateway with path",
			uri:      "https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG/metadata.json",
			expected: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		},
		// Edge cases
		{
			name:     "empty string",
			uri:      "",
			expected: "",
		},
		{
			name:     "non-IPFS URL",
			uri:      "https://example.com/image.png",
			expected: "",
		},
		{
			name:     "just ipfs://",
			uri:      "ipfs://",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCIDFromURI(tt.uri)
			if result != tt.expected {
				t.Errorf("extractCIDFromURI(%q) = %q, want %q", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestIsIPFSURI(t *testing.T) {
	tests := []struct {
		uri      string
		expected bool
	}{
		{"ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", true},
		{"ipfs://bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", true},
		{"https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", true},
		{"https://cloudflare-ipfs.com/ipfs/QmTest", true},
		{"https://example.com/image.png", false},
		{"http://localhost:8080/file.json", false},
		{"data:image/png;base64,abc123", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result := isIPFSURI(tt.uri)
			if result != tt.expected {
				t.Errorf("isIPFSURI(%q) = %v, want %v", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"timeout error string", fmt.Errorf("context deadline exceeded"), true},
		{"other error", fmt.Errorf("connection refused"), false},
		{"wrapped timeout", fmt.Errorf("failed: context deadline exceeded"), false}, // doesn't match exactly
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.err)
			if result != tt.expected {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestResolveURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "ipfs:// to gateway",
			uri:      "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
			expected: "https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		},
		{
			name:     "ipfs:// with path",
			uri:      "ipfs://QmTest/image.png",
			expected: "https://ipfs.io/ipfs/QmTest/image.png",
		},
		{
			name:     "already HTTP",
			uri:      "https://example.com/file.json",
			expected: "https://example.com/file.json",
		},
		{
			name:     "gateway URL unchanged",
			uri:      "https://ipfs.io/ipfs/QmTest",
			expected: "https://ipfs.io/ipfs/QmTest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveURI(tt.uri)
			if result != tt.expected {
				t.Errorf("resolveURI(%q) = %q, want %q", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestHasIPFSContent(t *testing.T) {
	tests := []struct {
		name     string
		metadata *indexer.TokenMetadata
		expected bool
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: false,
		},
		{
			name:     "empty metadata",
			metadata: &indexer.TokenMetadata{},
			expected: false,
		},
		{
			name: "has artifact URI",
			metadata: &indexer.TokenMetadata{
				ArtifactURI: "ipfs://QmTest",
			},
			expected: true,
		},
		{
			name: "has display URI",
			metadata: &indexer.TokenMetadata{
				DisplayURI: "ipfs://QmTest",
			},
			expected: true,
		},
		{
			name: "has thumbnail URI",
			metadata: &indexer.TokenMetadata{
				ThumbnailURI: "ipfs://QmTest",
			},
			expected: true,
		},
		{
			name: "has format URIs",
			metadata: &indexer.TokenMetadata{
				Formats: []indexer.Format{
					{URI: "ipfs://QmTest1"},
					{URI: "ipfs://QmTest2"},
				},
			},
			expected: true,
		},
		{
			name: "has all URIs",
			metadata: &indexer.TokenMetadata{
				ArtifactURI:  "ipfs://QmArtifact",
				DisplayURI:   "ipfs://QmDisplay",
				ThumbnailURI: "ipfs://QmThumb",
				Formats: []indexer.Format{
					{URI: "ipfs://QmFormat"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasIPFSContent(tt.metadata)
			if result != tt.expected {
				t.Errorf("hasIPFSContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCollectAssetURIs(t *testing.T) {
	tests := []struct {
		name         string
		metadata     *indexer.TokenMetadata
		expectedURIs []string
	}{
		{
			name:         "nil metadata",
			metadata:     nil,
			expectedURIs: []string{},
		},
		{
			name: "all unique URIs",
			metadata: &indexer.TokenMetadata{
				ArtifactURI:  "ipfs://QmArtifact",
				DisplayURI:   "ipfs://QmDisplay",
				ThumbnailURI: "ipfs://QmThumb",
				Formats: []indexer.Format{
					{URI: "ipfs://QmFormat1"},
					{URI: "ipfs://QmFormat2"},
				},
			},
			expectedURIs: []string{
				"ipfs://QmArtifact",
				"ipfs://QmDisplay",
				"ipfs://QmThumb",
				"ipfs://QmFormat1",
				"ipfs://QmFormat2",
			},
		},
		{
			name: "filters non-IPFS URIs",
			metadata: &indexer.TokenMetadata{
				ArtifactURI:  "ipfs://QmArtifact",
				DisplayURI:   "https://example.com/image.png", // non-IPFS
				ThumbnailURI: "data:image/png;base64,abc",     // non-IPFS
			},
			expectedURIs: []string{
				"ipfs://QmArtifact",
			},
		},
		{
			name: "deduplicates URIs",
			metadata: &indexer.TokenMetadata{
				ArtifactURI:  "ipfs://QmSame",
				DisplayURI:   "ipfs://QmSame", // duplicate
				ThumbnailURI: "ipfs://QmSame", // duplicate
			},
			expectedURIs: []string{
				"ipfs://QmSame",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := make(map[string]bool)
			collectAssetURIs(tt.metadata, seen)

			// Check count
			if len(seen) != len(tt.expectedURIs) {
				t.Errorf("collectAssetURIs() returned %d URIs, want %d", len(seen), len(tt.expectedURIs))
			}

			// Check each expected URI is present
			for _, uri := range tt.expectedURIs {
				if !seen[uri] {
					t.Errorf("collectAssetURIs() missing expected URI: %s", uri)
				}
			}
		})
	}
}

func TestCountAssets(t *testing.T) {
	tests := []struct {
		name     string
		metadata *indexer.TokenMetadata
		expected int
	}{
		{"nil", nil, 0},
		{"empty", &indexer.TokenMetadata{}, 0},
		{
			"one artifact",
			&indexer.TokenMetadata{ArtifactURI: "ipfs://Qm1"},
			1,
		},
		{
			"artifact and thumbnail same",
			&indexer.TokenMetadata{
				ArtifactURI:  "ipfs://QmSame",
				ThumbnailURI: "ipfs://QmSame",
			},
			1, // deduplicated
		},
		{
			"multiple unique",
			&indexer.TokenMetadata{
				ArtifactURI:  "ipfs://Qm1",
				DisplayURI:   "ipfs://Qm2",
				ThumbnailURI: "ipfs://Qm3",
			},
			3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countAssets(tt.metadata)
			if result != tt.expected {
				t.Errorf("countAssets() = %d, want %d", result, tt.expected)
			}
		})
	}
}
