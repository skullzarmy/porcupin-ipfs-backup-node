package uri

import "testing"

func TestIsIPFS(t *testing.T) {
	tests := []struct {
		uri      string
		expected bool
	}{
		{"ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", true},
		{"ipfs://bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", true},
		{"https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", true},
		{"https://cloudflare-ipfs.com/ipfs/QmTest", true},
		{"https://gateway.pinata.cloud/ipfs/QmTest", true},
		{"https://example.com/image.png", false},
		{"http://localhost:8080/file.json", false},
		{"data:image/png;base64,abc123", false},
		{"", false},
		{"ipfs://", true},
		{"IPFS://QmTest", false}, // Case sensitive
		{"/ipfs/QmTest", true},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result := IsIPFS(tt.uri)
			if result != tt.expected {
				t.Errorf("IsIPFS(%q) = %v, want %v", tt.uri, result, tt.expected)
			}
		})
	}
}
