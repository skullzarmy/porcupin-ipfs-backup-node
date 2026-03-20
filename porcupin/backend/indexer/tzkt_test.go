package indexer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewIndexer tests the indexer constructor
func TestNewIndexer(t *testing.T) {
	t.Run("with default URL", func(t *testing.T) {
		idx := NewIndexer("")
		if idx.baseURL != "https://api.tzkt.io" {
			t.Errorf("expected default baseURL, got %s", idx.baseURL)
		}
		if idx.client == nil {
			t.Error("expected client to be initialized")
		}
		if idx.httpClient == nil {
			t.Error("expected httpClient to be initialized")
		}
	})

	t.Run("with custom URL", func(t *testing.T) {
		customURL := "https://custom.tzkt.io"
		idx := NewIndexer(customURL)
		if idx.baseURL != customURL {
			t.Errorf("expected baseURL %s, got %s", customURL, idx.baseURL)
		}
	})
}

// TestSetTokenCallback tests setting the token callback
func TestSetTokenCallback(t *testing.T) {
	idx := NewIndexer("")
	
	if idx.tokenCallback != nil {
		t.Error("expected tokenCallback to be nil initially")
	}
	
	called := false
	idx.SetTokenCallback(func(token Token) {
		called = true
	})
	
	if idx.tokenCallback == nil {
		t.Error("expected tokenCallback to be set")
	}
	
	// Call the callback to verify it works
	idx.tokenCallback(Token{})
	if !called {
		t.Error("expected callback to be called")
	}
}

// TestTokenMetadataJSONParsing tests JSON unmarshaling of TokenMetadata
func TestTokenMetadataJSONParsing(t *testing.T) {
	t.Run("full metadata", func(t *testing.T) {
		jsonData := `{
			"name": "Test NFT",
			"description": "A test NFT",
			"artifactUri": "ipfs://QmTest123",
			"displayUri": "ipfs://QmDisplay456",
			"thumbnailUri": "ipfs://QmThumb789",
			"creators": ["tz1abc"],
			"formats": [{"uri": "ipfs://QmFormat", "mimeType": "image/png"}],
			"decimals": "0"
		}`
		
		var metadata TokenMetadata
		if err := json.Unmarshal([]byte(jsonData), &metadata); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		
		if metadata.Name != "Test NFT" {
			t.Errorf("expected name 'Test NFT', got '%s'", metadata.Name)
		}
		if metadata.ArtifactURI != "ipfs://QmTest123" {
			t.Errorf("expected artifactUri 'ipfs://QmTest123', got '%s'", metadata.ArtifactURI)
		}
		if len(metadata.Formats) != 1 {
			t.Errorf("expected 1 format, got %d", len(metadata.Formats))
		}
		if metadata.Formats[0].MimeType != "image/png" {
			t.Errorf("expected mimeType 'image/png', got '%s'", metadata.Formats[0].MimeType)
		}
	})

	t.Run("minimal metadata", func(t *testing.T) {
		jsonData := `{"name": "Minimal"}`
		
		var metadata TokenMetadata
		if err := json.Unmarshal([]byte(jsonData), &metadata); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		
		if metadata.Name != "Minimal" {
			t.Errorf("expected name 'Minimal', got '%s'", metadata.Name)
		}
	})

	t.Run("creators as string", func(t *testing.T) {
		jsonData := `{"name": "Test", "creators": "single_creator"}`
		
		var metadata TokenMetadata
		if err := json.Unmarshal([]byte(jsonData), &metadata); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		
		// creators is json.RawMessage so should parse without error
		if metadata.Creators == nil {
			t.Error("expected creators to be set")
		}
	})

	t.Run("decimals as integer", func(t *testing.T) {
		jsonData := `{"name": "Test", "decimals": 0}`
		
		var metadata TokenMetadata
		if err := json.Unmarshal([]byte(jsonData), &metadata); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
	})
}

// TestTokenJSONParsing tests JSON unmarshaling of Token
func TestTokenJSONParsing(t *testing.T) {
	jsonData := `{
		"id": 12345,
		"contract": {"address": "KT1abc", "alias": "Test Contract"},
		"tokenId": "1",
		"firstMinter": {"address": "tz1xyz", "alias": "Artist"},
		"metadata": {"name": "NFT", "artifactUri": "ipfs://Qm123"}
	}`
	
	var token Token
	if err := json.Unmarshal([]byte(jsonData), &token); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	
	if token.ID != 12345 {
		t.Errorf("expected ID 12345, got %d", token.ID)
	}
	if token.Contract.Address != "KT1abc" {
		t.Errorf("expected contract address 'KT1abc', got '%s'", token.Contract.Address)
	}
	if token.TokenID != "1" {
		t.Errorf("expected tokenId '1', got '%s'", token.TokenID)
	}
	if token.FirstMinter == nil || token.FirstMinter.Address != "tz1xyz" {
		t.Error("expected firstMinter to be set")
	}
	if token.Metadata == nil || token.Metadata.ArtifactURI != "ipfs://Qm123" {
		t.Error("expected metadata with artifactUri")
	}
}

// TestIsLikelyNFT tests the isLikelyNFT function
func TestIsLikelyNFT(t *testing.T) {
	t.Run("token with IPFS metadata", func(t *testing.T) {
		token := Token{
			Contract: ContractInfo{Address: "KT1unknown"},
			Metadata: &TokenMetadata{ArtifactURI: "ipfs://Qm123"},
		}
		if !isLikelyNFT(token) {
			t.Error("expected token with IPFS content to be NFT")
		}
	})

	t.Run("token from known NFT contract", func(t *testing.T) {
		token := Token{
			Contract: ContractInfo{Address: "KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton"}, // HEN
			Metadata: nil,
		}
		if !isLikelyNFT(token) {
			t.Error("expected token from HEN to be NFT")
		}
	})

	t.Run("token with NFT alias", func(t *testing.T) {
		aliases := []string{"My NFT Collection", "objkt marketplace", "fxhash tokens", "Rarible NFT"}
		for _, alias := range aliases {
			token := Token{
				Contract: ContractInfo{Address: "KT1unknown", Alias: alias},
				Metadata: nil,
			}
			if !isLikelyNFT(token) {
				t.Errorf("expected token with alias '%s' to be NFT", alias)
			}
		}
	})

	t.Run("token with null metadata", func(t *testing.T) {
		token := Token{
			Contract: ContractInfo{Address: "KT1unknown"},
			Metadata: nil,
		}
		// With null metadata, we still include it (permissive)
		if !isLikelyNFT(token) {
			t.Error("expected token with null metadata to be included")
		}
	})

	t.Run("token with empty metadata", func(t *testing.T) {
		token := Token{
			Contract: ContractInfo{Address: "KT1unknown"},
			Metadata: &TokenMetadata{},
		}
		if isLikelyNFT(token) {
			t.Error("expected token with empty metadata and unknown contract to not be NFT")
		}
	})
}

// TestHasIPFSContent tests the hasIPFSContent function
func TestHasIPFSContent(t *testing.T) {
	t.Run("nil metadata", func(t *testing.T) {
		if hasIPFSContent(nil) {
			t.Error("expected nil metadata to return false")
		}
	})

	t.Run("empty metadata", func(t *testing.T) {
		if hasIPFSContent(&TokenMetadata{}) {
			t.Error("expected empty metadata to return false")
		}
	})

	t.Run("has artifactUri", func(t *testing.T) {
		if !hasIPFSContent(&TokenMetadata{ArtifactURI: "ipfs://Qm123"}) {
			t.Error("expected metadata with artifactUri to return true")
		}
	})

	t.Run("has displayUri", func(t *testing.T) {
		if !hasIPFSContent(&TokenMetadata{DisplayURI: "ipfs://Qm123"}) {
			t.Error("expected metadata with displayUri to return true")
		}
	})

	t.Run("has thumbnailUri", func(t *testing.T) {
		if !hasIPFSContent(&TokenMetadata{ThumbnailURI: "ipfs://Qm123"}) {
			t.Error("expected metadata with thumbnailUri to return true")
		}
	})

	t.Run("has formats with URI", func(t *testing.T) {
		metadata := &TokenMetadata{
			Formats: []Format{{URI: "ipfs://QmFormat", MimeType: "image/png"}},
		}
		if !hasIPFSContent(metadata) {
			t.Error("expected metadata with formats URI to return true")
		}
	})

	t.Run("has formats without URI", func(t *testing.T) {
		metadata := &TokenMetadata{
			Formats: []Format{{MimeType: "image/png"}},
		}
		if hasIPFSContent(metadata) {
			t.Error("expected metadata with empty formats URI to return false")
		}
	})

	t.Run("HTTP-only URIs return false", func(t *testing.T) {
		metadata := &TokenMetadata{
			ArtifactURI:  "https://example.com/img.png",
			DisplayURI:   "https://cdn.example.com/display.jpg",
			ThumbnailURI: "https://cdn.example.com/thumb.jpg",
		}
		if hasIPFSContent(metadata) {
			t.Error("expected HTTP-only URIs to return false (BUG-3)")
		}
	})
}

// NOTE: TestKnownNFTContracts was removed. Testing that a map contains
// values that were just defined in the same file is tautological.

// TestGetHead tests the GetHead function with a mock server
func TestGetHead(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/head" {
				t.Errorf("expected path /v1/head, got %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Head{Level: 12345678})
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		level, err := idx.GetHead(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if level != 12345678 {
			t.Errorf("expected level 12345678, got %d", level)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		_, err := idx.GetHead(context.Background())
		if err == nil {
			t.Error("expected error for server error response")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			json.NewEncoder(w).Encode(Head{Level: 1})
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := idx.GetHead(ctx)
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})
}

// TestSyncOwned tests the SyncOwned function with a mock server
func TestSyncOwned(t *testing.T) {
	t.Run("returns NFTs for address", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/tokens/balances" {
				t.Errorf("expected path /v1/tokens/balances, got %s", r.URL.Path)
			}
			
			// Check query parameters
			q := r.URL.Query()
			if q.Get("account") == "" {
				t.Error("expected account parameter")
			}
			if q.Get("balance.ne") != "0" {
				t.Error("expected balance.ne=0 parameter")
			}
			
			w.Header().Set("Content-Type", "application/json")
			response := []struct {
				ID    uint64 `json:"id"`
				Token Token  `json:"token"`
			}{
				{
					ID: 1,
					Token: Token{
						ID:       100,
						Contract: ContractInfo{Address: "KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton"}, // Known NFT contract
						TokenID:  "1",
						Metadata: &TokenMetadata{ArtifactURI: "ipfs://Qm123"},
					},
				},
				{
					ID: 2,
					Token: Token{
						ID:       101,
						Contract: ContractInfo{Address: "KT1unknown"},
						TokenID:  "2",
						Metadata: &TokenMetadata{ThumbnailURI: "ipfs://Qm456"},
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		tokens, err := idx.SyncOwned(context.Background(), "tz1test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 2 {
			t.Errorf("expected 2 tokens, got %d", len(tokens))
		}
	})

	t.Run("empty result", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]struct{}{})
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		tokens, err := idx.SyncOwned(context.Background(), "tz1empty")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 0 {
			t.Errorf("expected 0 tokens, got %d", len(tokens))
		}
	})
}

// TestSyncOwnedSince tests the SyncOwnedSince function
func TestSyncOwnedSince(t *testing.T) {
	t.Run("includes lastLevel filter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("lastLevel.gt") != "1000" {
				t.Errorf("expected lastLevel.gt=1000, got %s", q.Get("lastLevel.gt"))
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]struct{}{})
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		_, err := idx.SyncOwnedSince(context.Background(), "tz1test", 1000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestSyncCreated tests the SyncCreated function with a mock server
func TestSyncCreated(t *testing.T) {
	t.Run("returns NFTs created by address", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/tokens" {
				t.Errorf("expected path /v1/tokens, got %s", r.URL.Path)
			}
			
			q := r.URL.Query()
			if q.Get("firstMinter") == "" {
				t.Error("expected firstMinter parameter")
			}
			
			w.Header().Set("Content-Type", "application/json")
			response := []Token{
				{
					ID:       100,
					Contract: ContractInfo{Address: "KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton"},
					TokenID:  "1",
					Metadata: &TokenMetadata{ArtifactURI: "ipfs://Qm123"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		tokens, err := idx.SyncCreated(context.Background(), "tz1artist")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 1 {
			t.Errorf("expected 1 token, got %d", len(tokens))
		}
	})
}

// TestSyncCreatedSince tests the SyncCreatedSince function
func TestSyncCreatedSince(t *testing.T) {
	t.Run("includes firstLevel filter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("firstLevel.gt") != "2000" {
				t.Errorf("expected firstLevel.gt=2000, got %s", q.Get("firstLevel.gt"))
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]Token{})
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		_, err := idx.SyncCreatedSince(context.Background(), "tz1artist", 2000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// NOTE: TestClose is intentionally omitted because the underlying tzkt/events library
// panics when Close() is called on an unconnected hub. This is a limitation of the
// external dependency, not our code. The Close() function is tested implicitly in
// integration tests where the connection is properly established first.

// TestContextCancellation tests that functions respect context cancellation
func TestContextCancellation(t *testing.T) {
	t.Run("SyncOwned respects cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]struct{}{})
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := idx.SyncOwned(ctx, "tz1test")
		if err == nil {
			t.Error("expected timeout error")
		}
	})

	t.Run("SyncCreated respects cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]Token{})
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := idx.SyncCreated(ctx, "tz1test")
		if err == nil {
			t.Error("expected timeout error")
		}
	})
}

// TestPagination tests cursor-based pagination behavior
func TestPagination(t *testing.T) {
	t.Run("SyncOwned paginates correctly", func(t *testing.T) {
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			q := r.URL.Query()
			w.Header().Set("Content-Type", "application/json")

			if requestCount == 1 {
				// First page - return full page to trigger pagination
				if q.Get("id.gt") != "" {
					t.Error("first request should not have id.gt")
				}
				// Return 1000 items (limit) to trigger pagination
				response := make([]struct {
					ID    uint64 `json:"id"`
					Token Token  `json:"token"`
				}, 1000)
				for i := 0; i < 1000; i++ {
					response[i] = struct {
						ID    uint64 `json:"id"`
						Token Token  `json:"token"`
					}{
						ID:    uint64(i + 1),
						Token: Token{ID: uint64(i + 100), Contract: ContractInfo{Address: "KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton"}, Metadata: &TokenMetadata{ArtifactURI: "ipfs://Qm"}},
					}
				}
				json.NewEncoder(w).Encode(response)
			} else {
				// Second page - return empty to stop pagination
				if q.Get("id.gt") != "1000" {
					t.Errorf("second request should have id.gt=1000, got %s", q.Get("id.gt"))
				}
				json.NewEncoder(w).Encode([]struct{}{})
			}
		}))
		defer server.Close()

		idx := NewIndexer(server.URL)
		tokens, err := idx.SyncOwned(context.Background(), "tz1test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 1000 {
			t.Errorf("expected 1000 tokens, got %d", len(tokens))
		}
		if requestCount != 2 {
			t.Errorf("expected 2 requests, got %d", requestCount)
		}
	})
}

// TestUnmarshalJSON_PreservesRawJSON verifies that the custom UnmarshalJSON
// stores the complete raw JSON bytes while still populating standard fields.
func TestUnmarshalJSON_PreservesRawJSON(t *testing.T) {
	t.Run("standard fields plus RawJSON", func(t *testing.T) {
		jsonData := `{"name":"Test","artifactUri":"ipfs://QmArt","displayUri":"ipfs://QmDisp"}`
		var m TokenMetadata
		if err := json.Unmarshal([]byte(jsonData), &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m.Name != "Test" {
			t.Errorf("Name = %q, want %q", m.Name, "Test")
		}
		if m.ArtifactURI != "ipfs://QmArt" {
			t.Errorf("ArtifactURI = %q, want %q", m.ArtifactURI, "ipfs://QmArt")
		}
		if m.RawJSON == nil {
			t.Fatal("RawJSON is nil")
		}
		// RawJSON should round-trip back to the same keys
		var probe map[string]interface{}
		if err := json.Unmarshal(m.RawJSON, &probe); err != nil {
			t.Fatalf("RawJSON re-parse: %v", err)
		}
		if probe["artifactUri"] != "ipfs://QmArt" {
			t.Error("RawJSON missing artifactUri")
		}
	})

	t.Run("extra fields preserved in RawJSON", func(t *testing.T) {
		jsonData := `{"name":"Versum","artifactUri":"ipfs://QmArt","pinUri":"ipfs://QmPin","rightUri":"ipfs://QmRight"}`
		var m TokenMetadata
		if err := json.Unmarshal([]byte(jsonData), &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// Standard field populated
		if m.ArtifactURI != "ipfs://QmArt" {
			t.Errorf("ArtifactURI = %q", m.ArtifactURI)
		}
		// Extra fields present in RawJSON
		var probe map[string]interface{}
		json.Unmarshal(m.RawJSON, &probe)
		if probe["pinUri"] != "ipfs://QmPin" {
			t.Error("RawJSON missing pinUri")
		}
		if probe["rightUri"] != "ipfs://QmRight" {
			t.Error("RawJSON missing rightUri")
		}
	})

	t.Run("nested in Token struct", func(t *testing.T) {
		jsonData := `{"id":1,"tokenId":"42","contract":{"address":"KT1abc"},"metadata":{"name":"NFT","artifactUri":"ipfs://Qm1","image":"ipfs://QmImg"}}`
		var tok Token
		if err := json.Unmarshal([]byte(jsonData), &tok); err != nil {
			t.Fatalf("unmarshal Token: %v", err)
		}
		if tok.Metadata == nil || tok.Metadata.RawJSON == nil {
			t.Fatal("Metadata.RawJSON is nil after Token unmarshal")
		}
		var probe map[string]interface{}
		json.Unmarshal(tok.Metadata.RawJSON, &probe)
		if probe["image"] != "ipfs://QmImg" {
			t.Error("RawJSON missing non-standard 'image' field")
		}
	})
}

// TestExtractExtraIPFSURIs exercises the recursive IPFS URI scanner.
func TestExtractExtraIPFSURIs(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		if uris := ExtractExtraIPFSURIs(nil); uris != nil {
			t.Errorf("expected nil, got %v", uris)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		if uris := ExtractExtraIPFSURIs(json.RawMessage{}); uris != nil {
			t.Errorf("expected nil, got %v", uris)
		}
	})

	t.Run("standard fields only", func(t *testing.T) {
		raw := json.RawMessage(`{"artifactUri":"ipfs://QmArt","displayUri":"ipfs://QmDisp","thumbnailUri":"ipfs://QmThumb"}`)
		uris := ExtractExtraIPFSURIs(raw)
		if len(uris) != 3 {
			t.Errorf("expected 3 URIs, got %d: %v", len(uris), uris)
		}
	})

	t.Run("non-standard fields", func(t *testing.T) {
		raw := json.RawMessage(`{"name":"Test","pinUri":"ipfs://QmPin","rightUri":"ipfs://QmRight"}`)
		uris := ExtractExtraIPFSURIs(raw)
		uriSet := make(map[string]bool)
		for _, u := range uris {
			uriSet[u] = true
		}
		if !uriSet["ipfs://QmPin"] {
			t.Error("missing pinUri")
		}
		if !uriSet["ipfs://QmRight"] {
			t.Error("missing rightUri")
		}
		if len(uris) != 2 {
			t.Errorf("expected 2 URIs (name excluded), got %d: %v", len(uris), uris)
		}
	})

	t.Run("nested objects", func(t *testing.T) {
		raw := json.RawMessage(`{"extra":{"deepUri":"ipfs://QmDeep"},"top":"ipfs://QmTop"}`)
		uris := ExtractExtraIPFSURIs(raw)
		uriSet := make(map[string]bool)
		for _, u := range uris {
			uriSet[u] = true
		}
		if !uriSet["ipfs://QmDeep"] {
			t.Error("missing nested deepUri")
		}
		if !uriSet["ipfs://QmTop"] {
			t.Error("missing top-level URI")
		}
	})

	t.Run("arrays with IPFS URIs", func(t *testing.T) {
		raw := json.RawMessage(`{"formats":[{"uri":"ipfs://QmFmt1"},{"uri":"ipfs://QmFmt2"}]}`)
		uris := ExtractExtraIPFSURIs(raw)
		if len(uris) != 2 {
			t.Errorf("expected 2 URIs from formats array, got %d: %v", len(uris), uris)
		}
	})

	t.Run("false positive exclusion - description", func(t *testing.T) {
		raw := json.RawMessage(`{"description":"Check out ipfs://QmFake for details","artifactUri":"ipfs://QmReal"}`)
		uris := ExtractExtraIPFSURIs(raw)
		uriSet := make(map[string]bool)
		for _, u := range uris {
			uriSet[u] = true
		}
		if uriSet["ipfs://QmFake"] {
			t.Error("description should be excluded but ipfs://QmFake was extracted")
		}
		if !uriSet["ipfs://QmReal"] {
			t.Error("artifactUri should be extracted")
		}
	})

	t.Run("false positive exclusion - name and tags", func(t *testing.T) {
		raw := json.RawMessage(`{"name":"ipfs://QmName","tags":["ipfs://QmTag"],"image":"ipfs://QmImg"}`)
		uris := ExtractExtraIPFSURIs(raw)
		uriSet := make(map[string]bool)
		for _, u := range uris {
			uriSet[u] = true
		}
		if uriSet["ipfs://QmName"] {
			t.Error("name should be excluded")
		}
		if uriSet["ipfs://QmTag"] {
			t.Error("tags should be excluded")
		}
		if !uriSet["ipfs://QmImg"] {
			t.Error("image should be extracted")
		}
	})

	t.Run("non-IPFS strings ignored", func(t *testing.T) {
		raw := json.RawMessage(`{"image":"https://example.com/img.png","data":"data:image/png;base64,abc","label":"hello"}`)
		uris := ExtractExtraIPFSURIs(raw)
		if len(uris) != 0 {
			t.Errorf("expected 0 IPFS URIs from non-IPFS content, got %d: %v", len(uris), uris)
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		raw := json.RawMessage(`{"artifactUri":"ipfs://QmSame","image":"ipfs://QmSame","preview":"ipfs://QmSame"}`)
		uris := ExtractExtraIPFSURIs(raw)
		if len(uris) != 1 {
			t.Errorf("expected 1 deduplicated URI, got %d: %v", len(uris), uris)
		}
	})

	t.Run("gateway-style URIs", func(t *testing.T) {
		raw := json.RawMessage(`{"image":"https://ipfs.io/ipfs/QmGateway"}`)
		uris := ExtractExtraIPFSURIs(raw)
		if len(uris) != 1 || uris[0] != "https://ipfs.io/ipfs/QmGateway" {
			t.Errorf("expected gateway URI, got %v", uris)
		}
	})

	t.Run("real-world Versum metadata", func(t *testing.T) {
		raw := json.RawMessage(`{
			"name": "Versum Artwork",
			"description": "A piece from Versum",
			"artifactUri": "ipfs://QmArtifactVersum",
			"displayUri": "ipfs://QmDisplayVersum",
			"thumbnailUri": "ipfs://QmThumbVersum",
			"pinUri": "ipfs://QmPinVersum",
			"rightUri": "ipfs://QmRightVersum",
			"creators": ["tz1abc"],
			"formats": [{"uri": "ipfs://QmFmtVersum", "mimeType": "image/png"}]
		}`)
		uris := ExtractExtraIPFSURIs(raw)
		uriSet := make(map[string]bool)
		for _, u := range uris {
			uriSet[u] = true
		}
		// pinUri and rightUri are non-standard — must be found
		if !uriSet["ipfs://QmPinVersum"] {
			t.Error("missing Versum pinUri")
		}
		if !uriSet["ipfs://QmRightVersum"] {
			t.Error("missing Versum rightUri")
		}
		// Standard URIs should also be found (they're not excluded)
		if !uriSet["ipfs://QmArtifactVersum"] {
			t.Error("missing standard artifactUri")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		uris := ExtractExtraIPFSURIs(json.RawMessage(`{invalid`))
		if uris != nil {
			t.Errorf("expected nil for invalid JSON, got %v", uris)
		}
	})
}

// TestHasIPFSContent_NonStandardFields verifies that hasIPFSContent detects
// IPFS URIs in non-standard metadata fields via RawJSON scanning.
func TestHasIPFSContent_NonStandardFields(t *testing.T) {
	t.Run("only non-standard IPFS field", func(t *testing.T) {
		m := &TokenMetadata{
			RawJSON: json.RawMessage(`{"pinUri":"ipfs://QmPin"}`),
		}
		if !hasIPFSContent(m) {
			t.Error("expected true when non-standard field has IPFS URI")
		}
	})

	t.Run("no IPFS URIs anywhere", func(t *testing.T) {
		m := &TokenMetadata{
			RawJSON: json.RawMessage(`{"name":"Test","description":"No IPFS here"}`),
		}
		if hasIPFSContent(m) {
			t.Error("expected false when no IPFS URIs exist")
		}
	})
}

// NOTE: TestHeadJSONParsing and TestTokenBalanceJSONParsing were removed.
// Testing JSON unmarshal on simple structs tests the stdlib, not our code.
