package indexer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

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

// TokenMetadata represents the metadata structure we expect from TZKT
type TokenMetadata struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	ArtifactURI  string          `json:"artifactUri"`
	DisplayURI   string          `json:"displayUri"`
	ThumbnailURI string          `json:"thumbnailUri"`
	Creators     json.RawMessage `json:"creators,omitempty"`  // Can be string or []string
	Formats      []Format        `json:"formats"`
	Decimals     json.RawMessage `json:"decimals,omitempty"` // Can be string or int
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
		
		log.Printf("SyncOwned: Requesting %s", reqURL)

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
			log.Printf("SyncOwned attempt %d failed, retrying in %v", attempt+1, backoff)
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

		log.Printf("SyncOwned: fetched %d balances, total NFTs so far: %d", len(balances), len(allTokens))

		if len(balances) < limit {
			break // Last page
		}
		
		time.Sleep(100 * time.Millisecond) // Rate limiting
	}

	log.Printf("SyncOwned complete: found %d NFTs for %s (since level %d)", len(allTokens), address, sinceLevel)
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
			log.Printf("SyncCreated attempt %d failed, retrying in %v", attempt+1, backoff)
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

		log.Printf("SyncCreated: fetched %d tokens (limit=%d), total NFTs so far: %d, lastId: %d, continuing: %v", 
			len(tokens), limit, len(allTokens), lastId, len(tokens) >= limit)

		if len(tokens) < limit {
			log.Printf("SyncCreated: Last page reached (got %d, limit was %d)", len(tokens), limit)
			break // Last page
		}
		
		log.Printf("SyncCreated: Fetching next page with id.gt=%d", lastId)
		time.Sleep(100 * time.Millisecond) // Rate limiting
	}

	log.Printf("SyncCreated complete: found %d NFTs for %s (since level %d)", len(allTokens), address, sinceLevel)
	return allTokens, nil
}

// FetchRawMetadataURI retrieves the raw IPFS URI for a token's metadata
func (i *Indexer) FetchRawMetadataURI(ctx context.Context, contractAddress string, tokenId string) (string, error) {
	// 1. Get contract storage schema to find `token_metadata` bigmap ID.
	var contract struct {
		BigMapIDs struct {
			TokenMetadata uint64 `json:"token_metadata"`
		} `json:"bigmaps"`
	}
	
	if err := i.get(ctx, fmt.Sprintf("/v1/contracts/%s", contractAddress), nil, &contract); err != nil {
		return "", fmt.Errorf("failed to get contract info: %w", err)
	}

	if contract.BigMapIDs.TokenMetadata == 0 {
		return "", fmt.Errorf("token_metadata bigmap not found for contract %s", contractAddress)
	}

	// 2. Query the BigMap for the specific token
	var keys []struct {
		Value struct {
			TokenInfo map[string]string `json:"token_info"`
		} `json:"value"`
	}

	filters := map[string]string{
		"bigmap": fmt.Sprintf("%d", contract.BigMapIDs.TokenMetadata),
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
			log.Printf("Recovered from websocket panic: %v", r)
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
					log.Printf("WebSocket subscription confirmed, state: %v", msg.State)
					continue
				}
				
				log.Printf("Received token balance update: type=%d, state=%v", msg.Type, msg.State)
				
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
// This is more permissive than hasIPFSContent - it includes tokens with null metadata
// since we can try to fetch metadata from chain
func isLikelyNFT(t Token) bool {
	// If we already have metadata with IPFS content, definitely include
	if t.Metadata != nil && hasIPFSContent(t.Metadata) {
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

// hasIPFSContent checks if metadata contains any IPFS URIs to backup
func hasIPFSContent(m *TokenMetadata) bool {
	if m == nil {
		return false
	}
	// Check main URIs
	if m.ArtifactURI != "" || m.DisplayURI != "" || m.ThumbnailURI != "" {
		return true
	}
	// Check formats array
	for _, f := range m.Formats {
		if f.URI != "" {
			return true
		}
	}
	return false
}
