package ipfs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNodePinAndVerify(t *testing.T) {
	// Create a temporary directory for the test IPFS repo
	tmpDir, err := os.MkdirTemp("", "ipfs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := filepath.Join(tmpDir, "ipfs")
	
	// Create and start node
	node, err := NewNode(repoPath)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := node.Start(ctx); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer node.Stop()

	// Add some test content
	testContent := []byte("Hello, IPFS! This is a test content for verification.")
	cid, err := node.Add(ctx, bytes.NewReader(testContent))
	if err != nil {
		t.Fatalf("Failed to add content: %v", err)
	}
	t.Logf("Added content with CID: %s", cid)

	// Test IsPinned
	pinned, err := node.IsPinned(ctx, cid)
	if err != nil {
		t.Fatalf("Failed to check pin status: %v", err)
	}
	if !pinned {
		t.Error("Content should be pinned after Add")
	}

	// Test Verify
	result := node.Verify(ctx, cid, 30*time.Second)
	if !result.IsPinned {
		t.Error("Verify should report content as pinned")
	}
	if !result.IsAvailable {
		t.Error("Verify should report content as available")
	}
	if result.Size != int64(len(testContent)) {
		t.Errorf("Size mismatch: expected %d, got %d", len(testContent), result.Size)
	}
	if result.Error != "" {
		t.Errorf("Unexpected error in verify result: %s", result.Error)
	}

	// Test Cat
	data, mimeType, err := node.Cat(ctx, cid, 1024)
	if err != nil {
		t.Fatalf("Failed to cat content: %v", err)
	}
	if !bytes.Equal(data, testContent) {
		t.Error("Cat returned different content than what was added")
	}
	t.Logf("Cat returned %d bytes with mime type: %s", len(data), mimeType)

	// Test Unpin and verify again
	if err := node.Unpin(ctx, cid); err != nil {
		t.Fatalf("Failed to unpin: %v", err)
	}

	pinned, err = node.IsPinned(ctx, cid)
	if err != nil {
		t.Fatalf("Failed to check pin status after unpin: %v", err)
	}
	if pinned {
		t.Error("Content should not be pinned after Unpin")
	}
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "JPEG",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0},
			expected: "image/jpeg",
		},
		{
			name:     "PNG",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expected: "image/png",
		},
		{
			name:     "GIF",
			data:     []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61},
			expected: "image/gif",
		},
		{
			name:     "JSON",
			data:     []byte(`{"key": "value"}`),
			expected: "application/json",
		},
		{
			name:     "HTML",
			data:     []byte(`<html><body>Hello</body></html>`),
			expected: "text/html",
		},
		{
			name:     "Unknown",
			data:     []byte{0x00, 0x01, 0x02, 0x03},
			expected: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectMimeType(tt.data)
			if result != tt.expected {
				t.Errorf("detectMimeType() = %s, expected %s", result, tt.expected)
			}
		})
	}
}
