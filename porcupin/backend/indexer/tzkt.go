package indexer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	ipfsuri "porcupin/backend/uri"

	"github.com/dipdup-net/go-lib/tzkt/api"
	"github.com/dipdup-net/go-lib/tzkt/events"
)

// Indexer handles interactions with the TZKT API
type Indexer struct {
	client        *api.API
	httpClient    *http.Client
	baseURL       string
	events        *events.TzKT
	tokenCallback func(Token) // Callback for new tokens from WebSocket
}

// NewIndexer creates a new TZKT indexer instance
func NewIndexer(baseURL string) *Indexer {
	if baseURL == "" {
		baseURL = "https://api.tzkt.io"
	}
	
	// Create API client with default HTTP client
	client := api.New(baseURL)

	return &Indexer{
		client:     client,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
		events:     events.NewTzKT(fmt.Sprintf("%s/v1/ws", baseURL)),
	}
}

// SetTokenCallback sets the callback function for new tokens
func (i *Indexer) SetTokenCallback(cb func(Token)) {
	i.tokenCallback = cb
}

// TokenMetadata represents the metadata structure we expect from TZKT.
// The custom UnmarshalJSON stores the complete raw JSON in RawJSON so that
// non-standard fields (e.g. Versum's pinUri, fxhash's generativeUri) can be
// scanned for additional IPFS URIs.
type TokenMetadata struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	ArtifactURI  string          `json:"artifactUri"`
	DisplayURI   string          `json:"displayUri"`
	ThumbnailURI string          `json:"thumbnailUri"`
	Creators     json.RawMessage `json:"creators,omitempty"`  // Can be string or []string
	Formats      []Format        `json:"formats"`
	Decimals     json.RawMessage `json:"decimals,omitempty"` // Can be string or int

	// RawJSON stores the complete original JSON bytes for this metadata object.
	// Populated automatically by UnmarshalJSON. Used by ExtractExtraIPFSURIs
	// to discover IPFS URIs in non-standard fields.
	RawJSON json.RawMessage `json:"-"`
}

// UnmarshalJSON implements json.Unmarshaler. It performs standard field
// decoding AND stores the complete raw JSON bytes so callers can scan
// for IPFS URIs in non-standard fields.
func (m *TokenMetadata) UnmarshalJSON(data []byte) error {
	// Use an alias type to prevent infinite recursion — the alias has
	// the same fields but no methods, so json uses default struct decoding.
	type Alias TokenMetadata
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*m = TokenMetadata(alias)

	// Store a copy of the complete raw JSON.
	m.RawJSON = make(json.RawMessage, len(data))
	copy(m.RawJSON, data)

	return nil
}

type Format struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
}

// Token represents a simplified token object from TZKT
type Token struct {
	ID          uint64         `json:"id"`
	Contract    ContractInfo   `json:"contract"`
	TokenID     string         `json:"tokenId"`
	FirstMinter *MinterInfo    `json:"firstMinter,omitempty"`
	Metadata    *TokenMetadata `json:"metadata"`
}

type ContractInfo struct {
	Address string `json:"address"`
	Alias   string `json:"alias,omitempty"`
}

type MinterInfo struct {
	Address string `json:"address"`
	Alias   string `json:"alias,omitempty"`
}

// TokenBalance represents a token balance entry
type TokenBalance struct {
	Token Token `json:"token"`
}

// Head represents the current blockchain head
type Head struct {
	Level int64 `json:"level"`
}

// GetHead fetches the current blockchain head level
func (i *Indexer) GetHead(ctx context.Context) (int64, error) {
	var head Head
	if err := i.get(ctx, "/v1/head", nil, &head); err != nil {
		return 0, fmt.Errorf("failed to get head: %w", err)
	}
	return head.Level, nil
}

// get performs a GET request to the TZKT API
func (i *Indexer) get(ctx context.Context, endpoint string, params map[string]string, v interface{}) error {
	u, err := url.Parse(fmt.Sprintf("%s%s", i.baseURL, endpoint))
	if err != nil {
		return err
	}

	q := u.Query()
	for k, val := range params {
		q.Set(k, val)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

// SyncOwned fetches all NFTs owned by an account with cursor-based pagination
// Uses lastId pagination (recommended by TZKT) instead of offset for reliable results
// If sinceLevel > 0, only fetches tokens updated after that blockchain level
func (i *Indexer) SyncOwned(ctx context.Context, address string) ([]Token, error) {
	return i.SyncOwnedSince(ctx, address, 0)
}

// SyncOwnedSince fetches NFTs owned by an account, optionally only those updated after sinceLevel
func (i *Indexer) SyncOwnedSince(ctx context.Context, address string, sinceLevel int64) ([]Token, error) {
	var allTokens []Token
	var lastId uint64 = 0
	limit := 1000 // TZKT recommended batch size

	// Use a custom client with longer timeout for this operation
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	for {
		// Build URL with cursor-based pagination using id.gt (greater than lastId)
		// balance.ne=0 ensures we only get tokens the account actually holds
		reqURL := fmt.Sprintf("%s/v1/tokens/balances?account=%s&balance.ne=0&limit=%d&sort.asc=id",
			i.baseURL, address, limit)
		
		if lastId > 0 {
			reqURL += fmt.Sprintf("&id.gt=%d", lastId)
		}
		
		// Filter by lastLevel if we're doing an incremental sync
		if sinceLevel > 0 {
			reqURL += fmt.Sprintf("&lastLevel.gt=%d", sinceLevel)
		}
		
		slog.Debug("SyncOwned: requesting page", "url", reqURL)

		var balances []struct {
			ID    uint64 `json:"id"` // Balance record ID for pagination cursor
			Token Token  `json:"token"`
		}

		// Retry logic with exponential backoff
		var resp *http.Response
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			req, reqErr := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
			if reqErr != nil {
				return allTokens, fmt.Errorf("failed to create request: %w", reqErr)
			}
			resp, err = client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				break
			}
			if resp != nil {
				resp.Body.Close()
			}
			backoff := time.Second * time.Duration(1<<uint(attempt))
			slog.Warn("SyncOwned attempt failed, retrying", "attempt", attempt+1, "backoff", backoff)
			time.Sleep(backoff)
		}
		if err != nil {
			return allTokens, fmt.Errorf("failed to fetch owned tokens after retries: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return allTokens, fmt.Errorf("failed to fetch owned tokens: status %d", resp.StatusCode)
		}

		if err := json.NewDecoder(resp.Body).Decode(&balances); err != nil {
			resp.Body.Close()
			return allTokens, fmt.Errorf("failed to decode owned tokens: %v", err)
		}
		resp.Body.Close()

		if len(balances) == 0 {
			break
		}

		for _, b := range balances {
			if isLikelyNFT(b.Token) {
				allTokens = append(allTokens, b.Token)
			}
			lastId = b.ID
		}

		slog.Debug("SyncOwned: fetched balances", "count", len(balances), "total_nfts", len(allTokens))

		if len(balances) < limit {
			break // Last page
		}
		
		time.Sleep(100 * time.Millisecond) // Rate limiting
	}

	slog.Info("SyncOwned complete", "nft_count", len(allTokens), "address", address, "since_level", sinceLevel)
	return allTokens, nil
}

// SyncCreated fetches all NFTs created by an account (firstMinter) with cursor-based pagination
// Uses lastId pagination (recommended by TZKT) instead of offset for reliable results
func (i *Indexer) SyncCreated(ctx context.Context, address string) ([]Token, error) {
	return i.SyncCreatedSince(ctx, address, 0)
}

// SyncCreatedSince fetches NFTs created by an account, optionally only those created after sinceLevel
func (i *Indexer) SyncCreatedSince(ctx context.Context, address string, sinceLevel int64) ([]Token, error) {
	var allTokens []Token
	var lastId uint64 = 0
	limit := 1000 // TZKT recommended batch size

	// Use a custom client with longer timeout
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	for {
		// Build URL with cursor-based pagination using id.gt (greater than lastId)
		// Don't use select parameter - get full response for proper parsing
		reqURL := fmt.Sprintf("%s/v1/tokens?firstMinter=%s&limit=%d&sort.asc=id",
			i.baseURL, address, limit)

		if lastId > 0 {
			reqURL += fmt.Sprintf("&id.gt=%d", lastId)
		}

		// Filter by firstLevel if we're doing an incremental sync
		if sinceLevel > 0 {
			reqURL += fmt.Sprintf("&firstLevel.gt=%d", sinceLevel)
		}

		var tokens []Token

		// Retry logic with exponential backoff
		var resp *http.Response
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			req, reqErr := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
			if reqErr != nil {
				return allTokens, fmt.Errorf("failed to create request: %w", reqErr)
			}
			resp, err = client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				break
			}
			if resp != nil {
				resp.Body.Close()
			}
			backoff := time.Second * time.Duration(1<<uint(attempt))
			slog.Warn("SyncCreated attempt failed, retrying", "attempt", attempt+1, "backoff", backoff)
			time.Sleep(backoff)
		}
		if err != nil {
			return allTokens, fmt.Errorf("failed to fetch created tokens after retries: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return allTokens, fmt.Errorf("failed to fetch created tokens: status %d", resp.StatusCode)
		}

		if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
			resp.Body.Close()
			return allTokens, fmt.Errorf("failed to decode created tokens: %v", err)
		}
		resp.Body.Close()

		if len(tokens) == 0 {
			break
		}

		for _, t := range tokens {
			// Include tokens that are likely NFTs
			if isLikelyNFT(t) {
				allTokens = append(allTokens, t)
			}
			lastId = t.ID // Update cursor
		}

		slog.Debug("SyncCreated: fetched tokens", "count", len(tokens), "limit", limit, "total_nfts", len(allTokens), "last_id", lastId, "continuing", len(tokens) >= limit)

		if len(tokens) < limit {
			slog.Debug("SyncCreated: last page reached", "count", len(tokens), "limit", limit)
			break // Last page
		}
		
		slog.Debug("SyncCreated: fetching next page", "id_gt", lastId)
		time.Sleep(100 * time.Millisecond) // Rate limiting
	}

	slog.Info("SyncCreated complete", "nft_count", len(allTokens), "address", address, "since_level", sinceLevel)
	return allTokens, nil
}

// FetchRawMetadataURI retrieves the raw IPFS URI for a token's metadata
func (i *Indexer) FetchRawMetadataURI(ctx context.Context, contractAddress string, tokenId string) (string, error) {
	// 1. Get contract storage schema to find `token_metadata` bigmap ID.
	// Try to get it from the dedicated bigmaps endpoint which is more reliable
	bigMapID, err := i.GetTokenMetadataBigMapID(ctx, contractAddress)
	if err != nil {
		return "", fmt.Errorf("failed to get token_metadata bigmap ID: %w", err)
	}

	// 2. Query the BigMap for the specific token
	var keys []struct {
		Value struct {
			TokenInfo map[string]string `json:"token_info"`
		} `json:"value"`
	}

	filters := map[string]string{
		"bigmap": fmt.Sprintf("%d", bigMapID),
		"key":    tokenId,
	}

	if err := i.get(ctx, "/v1/bigmaps/keys", filters, &keys); err != nil {
		return "", fmt.Errorf("failed to fetch bigmap key: %w", err)
	}

	if len(keys) == 0 {
		return "", fmt.Errorf("metadata not found in bigmap")
	}

	// 3. Extract and decode the URI
	hexURI, ok := keys[0].Value.TokenInfo[""]
	if !ok {
		hexURI, ok = keys[0].Value.TokenInfo["metadata"]
		if !ok {
			return "", fmt.Errorf("no URI found in token_info")
		}
	}

	bytesURI, err := hex.DecodeString(hexURI)
	if err != nil {
		return "", fmt.Errorf("failed to decode hex URI: %w", err)
	}

	return string(bytesURI), nil
}

// GetTokenMetadataBigMapID finds the token_metadata bigmap ID for a contract
func (i *Indexer) GetTokenMetadataBigMapID(ctx context.Context, contractAddress string) (uint64, error) {
	// Query all active bigmaps for the contract
	// We fetch minimal fields to be efficient
	var bigmaps []struct {
		Ptr  uint64   `json:"ptr"`
		Path string   `json:"path"`
		Tags []string `json:"tags"`
	}

	params := map[string]string{
		"active": "true",
		"select": "ptr,path,tags",
	}

	endpoint := fmt.Sprintf("/v1/contracts/%s/bigmaps", contractAddress)
	if err := i.get(ctx, endpoint, params, &bigmaps); err != nil {
		return 0, fmt.Errorf("failed to fetch contract bigmaps: %w", err)
	}

	for _, bm := range bigmaps {
		// Check path first (most reliable)
		if bm.Path == "token_metadata" {
			return bm.Ptr, nil
		}
		
		// Check tags as fallback
		for _, tag := range bm.Tags {
			if tag == "token_metadata" {
				return bm.Ptr, nil
			}
		}
	}

	return 0, fmt.Errorf("token_metadata bigmap not found for contract %s", contractAddress)
}

// Listen subscribes to real-time updates for the given address
// This function blocks until the context is cancelled or the connection closes
func (i *Indexer) Listen(ctx context.Context, address string) error {
	// Connect with context for cancellation
	if err := i.events.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Subscribe to TokenBalances (for ownership changes)
	if err := i.events.SubscribeToTokenBalances(address, "", ""); err != nil {
		i.events.Close()
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	
	// Block on handleEvents - it will return when the connection closes
	err := i.handleEvents(ctx)
	
	// Always close on exit to clean up
	i.events.Close()
	
	return err
}

func (i *Indexer) handleEvents(ctx context.Context) (err error) {
	// Recover from any panics in event handling
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("websocket panic: %v", r)
			slog.Error("recovered from websocket panic", "error", r)
		}
	}()

	msgChan := i.events.Listen()
	if msgChan == nil {
		return fmt.Errorf("listen returned nil channel")
	}
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgChan:
			// Channel closed - connection died
			if !ok {
				return fmt.Errorf("websocket channel closed")
			}
			
			// Check if still connected before processing
			if !i.events.IsConnected() {
				return fmt.Errorf("websocket disconnected")
			}
			
			switch msg.Channel {
			case events.ChannelTokenBalances:
				// Ignore nil or empty balance updates (connection keep-alives)
				if msg.Body == nil {
					continue
				}
				
				// Check if body has actual content (not just empty state message)
				if msg.Type == 0 {
					// Type 0 is state message (subscription confirmation), not data
					slog.Info("WebSocket subscription confirmed", "state", msg.State)
					continue
				}
				
				slog.Debug("received token balance update", "type", msg.Type, "state", msg.State)
				
				// Only fire callback for actual data messages (type 1)
				if i.tokenCallback != nil && msg.Type == 1 {
					i.tokenCallback(Token{})
				}
			}
		}
	}
}

// Close closes the indexer connections
func (i *Indexer) Close() error {
	return i.events.Close()
}

// Known NFT contract addresses on Tezos
var knownNFTContracts = map[string]bool{
	"KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton": true, // HEN (hic et nunc)
	"KT1U6EHmNxJTkvaWJ4ThczG4FSDaHC21ssvi": true, // fxhash GENTK v1
	"KT1KEa8z6vWXDJrVqtMrAeDVzsvxat3kHaCE": true, // fxhash GENTK v2
	"KT1GtbuswcNMGhHF2TSuH1Yfaqn16do8Qtva": true, // fxhash articles
	"KT18pVpRXKPY2c4U2yFEGSH3ZnhB2kL8kwXS": true, // Rarible
	"KT1EFS5kqVYLvM8FaX1CftJ8FT4U6MHdJxPn": true, // Objkt.com v1
	"KT1WvzYHCNBvDSdwafTHv7nJ1dWmZ8GCYuuC": true, // Objkt.com v2
	"KT1LjmAdYQCLBjwv4S2oFkEzyHVkomAf5MrW": true, // Versum
	"KT1SLWhfqPtQq7f4zLomh8DYjxaLeAgH72E6": true, // 8bidou
	"KT1MxDwChiDwd2CC7QDyAg1eLDJZdJCn7wTR": true, // TypedArt
	"KT1NVvPsNDChrLRH5K2cy6Sc9r1uuUwdiZQd": true, // akaSwap
	"KT1AFq5XorPduoYyWxs5gEyrFK6fVjJVbtCj": true, // akaDAO
	"KT1EpGgjQs73QfFJs9z7m1Mxm5MTnpC2tqse": true, // Kalamint
	"KT1ViVwoVfGSCsDaxjwoovejm1aYSGz7s2TZ": true, // TzColors
}

// isLikelyNFT determines if a token is likely an NFT worth backing up
// This is more permissive than HasIPFSContent - it includes tokens with null metadata
// since we can try to fetch metadata from chain
func isLikelyNFT(t Token) bool {
	// If we already have metadata with IPFS content, definitely include
	if t.Metadata != nil && HasIPFSContent(t.Metadata) {
		return true
	}
	
	// Check if from a known NFT contract
	if knownNFTContracts[t.Contract.Address] {
		return true
	}
	
	// Check contract alias for common NFT platforms
	alias := strings.ToLower(t.Contract.Alias)
	if strings.Contains(alias, "nft") ||
		strings.Contains(alias, "objkt") ||
		strings.Contains(alias, "fxhash") ||
		strings.Contains(alias, "hen") ||
		strings.Contains(alias, "hic et nunc") ||
		strings.Contains(alias, "versum") ||
		strings.Contains(alias, "rarible") ||
		strings.Contains(alias, "kalamint") ||
		strings.Contains(alias, "typed") ||
		strings.Contains(alias, "akaswap") ||
		strings.Contains(alias, "8bidou") {
		return true
	}
	
	// If metadata is null but contract looks like it could be an NFT platform, include it
	// We'll try to fetch metadata from chain later
	if t.Metadata == nil {
		// Include if it's from any FA2 contract (most NFTs are FA2)
		// We can filter out non-NFTs later during processing
		return true
	}
	
	return false
}

// HasIPFSContent checks if metadata contains any IPFS URIs to backup.
func HasIPFSContent(m *TokenMetadata) bool {
	if m == nil {
		return false
	}
	// Check standard TZIP-21 URIs — only IPFS URIs count as backupable content.
	// HTTP-only URIs must NOT pass this gate; they would enter the DB as "pending"
	// and fail during backup with "Not an IPFS URI" (BUG-3).
	if ipfsuri.IsIPFS(m.ArtifactURI) || ipfsuri.IsIPFS(m.DisplayURI) || ipfsuri.IsIPFS(m.ThumbnailURI) {
		return true
	}
	// Check formats array
	for _, f := range m.Formats {
		if ipfsuri.IsIPFS(f.URI) {
			return true
		}
	}
	// Check for IPFS URIs in non-standard metadata fields
	if len(ExtractExtraIPFSURIs(m.RawJSON)) > 0 {
		return true
	}
	return false
}

// extractionExcludeKeys contains metadata field names whose values are
// human-readable text, not URIs. Scanning these would produce false positives
// (e.g. an artist who writes about IPFS in their description).
var extractionExcludeKeys = map[string]bool{
	"name":        true,
	"description": true,
	"symbol":      true,
	"language":    true,
	"rights":      true,
	"date":        true,
	"tags":        true,
	"attributes":  true,
	"creators":    true,
	"minter":      true,
	"mintingTool": true,
	"type":        true,
	"mimeType":    true,
	"fileName":    true,
	"dimensions":  true,
	"decimals":    true,
}

// ExtractExtraIPFSURIs walks rawJSON recursively and returns all unique
// IPFS URIs found in string values, skipping known text-content keys
// (name, description, etc.) to avoid false positives.
func ExtractExtraIPFSURIs(rawJSON json.RawMessage) []string {
	if len(rawJSON) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var walk func(key string, v interface{})
	walk = func(key string, v interface{}) {
		switch val := v.(type) {
		case string:
			if extractionExcludeKeys[key] {
				return
			}
			if ipfsuri.IsIPFS(val) {
				seen[val] = true
			}
		case map[string]interface{}:
			for k, child := range val {
				walk(k, child)
			}
		case []interface{}:
			for _, item := range val {
				walk(key, item)
			}
		}
	}

	var root interface{}
	if err := json.Unmarshal(rawJSON, &root); err != nil {
		return nil
	}
	walk("", root)

	if len(seen) == 0 {
		return nil
	}

	result := make([]string, 0, len(seen))
	for uri := range seen {
		result = append(result, uri)
	}
	return result
}
