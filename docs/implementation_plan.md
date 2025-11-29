# Implementation Plan - Porcupin Tezos Backup Node

# Goal Description
Build "Porcupin", a native desktop application and headless node for backing up Tezos NFT data to IPFS. The goal is to preserve NFT history by ensuring metadata and assets are pinned and available on the IPFS network.

## User Review Required
> [!IMPORTANT]
> **Technology Choice**: We are using **Go (Golang)** for the backend and **Wails + React** for the desktop application. This ensures high performance, native binaries, and a modern UI.
> **IPFS Strategy**: We will use the **Kubo** (go-ipfs) libraries to embed an IPFS node directly into the application. This provides a self-contained experience but increases the binary size.
> **Data Storage**: SQLite will be used for local state management.

## Proposed Changes

### 1. Project Initialization
#### [NEW] [go.mod](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/go.mod)
- Initialize Go module `github.com/antigravity/porcupin`.
- Add dependencies: `wails`, `gorm`, `kubo`, `tzkt-go`.

#### [NEW] [wails.json](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/wails.json)
- Initialize Wails project configuration.

### 2. Backend Implementation (Go)

#### [NEW] [backend/internal/db/db.go](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/backend/internal/db/db.go)
- Setup SQLite connection using GORM.
- Define models: `Wallet`, `NFT`, `Asset`.

#### [NEW] [backend/internal/ipfs/node.go](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/backend/internal/ipfs/node.go)
- Implement embedded IPFS node startup/shutdown.
- Implement `Pin(cid string, timeout time.Duration)` with context deadline (2min default).
- Implement streaming download to disk (avoid RAM buffering for large files).
- Implement `MaxFileSize` check before pinning.
- Implement bandwidth rate limiter using `golang.org/x/time/rate`.

#### [NEW] [backend/internal/indexer/tzkt.go](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/backend/internal/indexer/tzkt.go)
- Implement `SyncOwned(address string)` using `/v1/tokens/balances` (filter by `artifactUri`).
- Implement `SyncCreated(address string)` using `/v1/tokens` (filter by `firstMinter`).
- Implement `FetchRawMetadataURI(contract, tokenId)` by querying `token_metadata` BigMap and hex-decoding `token_info[""]`.
- Implement rate limit handler: detect 429 errors, respect `Retry-After` header.
- Implement `Listen(address string)` using TZKT Websockets.

#### [NEW] [backend/internal/core/backup.go](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/backend/internal/core/backup.go)
- Implement the `Worker Pool` pattern for concurrent asset processing.
- Orchestrate fetching metadata -> parsing -> pinning assets.
- Implement exponential backoff for network retries.
- Implement metadata validation: 1MB size limit, basic JSON schema check.
- Implement disk space monitoring: pause if free space < 5GB.
- Distinguish `FAILED` vs `FAILED_UNAVAILABLE` based on timeout vs other errors.

### 3. Frontend Implementation (React)

#### [NEW] [frontend/src/App.jsx](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/frontend/src/App.jsx)
- Main layout with Sidebar and Dashboard.

#### [NEW] [frontend/src/components/Dashboard.jsx](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/frontend/src/components/Dashboard.jsx)
- Display stats (Total Pinned, Active Wallets).

#### [NEW] [frontend/src/components/WalletManager.jsx](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/frontend/src/components/WalletManager.jsx)
- Form to add/remove wallets.
- List of tracked wallets with sync status.
- **NEVER display NFT content/thumbnails** (XSS risk). Only show count and status.

---

### 4. Security & Configuration

#### [NEW] [backend/internal/api/auth.go](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/backend/internal/api/auth.go)
- Implement Basic Auth middleware for web dashboard (optional via config).

#### [NEW] [backend/config/config.go](file:///Users/joepeterson/.gemini/antigravity/playground/glacial-filament/backend/config/config.go)
- Define configuration struct with defaults:
    - `MaxFileSize: 5GB`
    - `PinTimeout: 2min`
    - `MaxConcurrency: 5`
    - `BindAddress: 127.0.0.1:8080`
    - `EnableAuth: false`

## Verification Plan

### Automated Tests
- **Unit Tests**: Run `go test ./...` to verify backend logic (DB, Indexer parsing, IPFS mocking).
- **Integration Tests**: Create a test that mocks the TZKT API and verifies that the `BackupManager` correctly creates `Asset` records in the DB.

### Manual Verification
1.  **Build**: Run `wails build` to ensure the app compiles for the host OS.
2.  **Run**: Launch the app.
3.  **Add Wallet**: Add a known Tezos wallet address (e.g., a curator or artist).
4.  **Verify Sync**: Check logs/dashboard to see if NFTs are detected.
5.  **Verify Pinning**: Check the "Pinned Size" metric increasing.
6.  **IPFS Check**: Use a public gateway (e.g., `ipfs.io/ipfs/<CID>`) to verify that a file pinned by Porcupin is accessible (if the node is public/connected).
