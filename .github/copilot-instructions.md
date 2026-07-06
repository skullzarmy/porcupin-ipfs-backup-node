# Porcupin Copilot Instructions

## Project Overview

Porcupin is a Tezos NFT preservation app that pins NFT assets to IPFS. It uses **Wails v2** (Go + React) for the desktop app with an embedded Kubo IPFS node. **It's designed as a "set and forget" app** - once you add wallets, it automatically syncs and pins NFT assets.

## Architecture

### Two Codebases

-   **`/porcupin`** - The active Wails desktop application (primary development target)
-   **`/backend`** - Standalone Go backend (future headless/Docker deployment)

### Core Data Flow

```
TZKT API → Indexer → Database (SQLite) → BackupService → BackupManager → IPFS Node
                                              ↑
                                    TZKT WebSocket (real-time updates)
```

### Automatic Sync Architecture

The app automatically syncs on startup and watches for new NFTs:

1. **On Startup**: `BackupService.Start()` is called
2. **Catch-up Sync**: Syncs all wallets that haven't been synced recently
3. **Watch Mode**: WebSocket connections for real-time token balance updates
4. **Retry Worker**: Background goroutine retries failed assets every 2 minutes
5. **Health Checks**: Periodic IPFS health checks (every 5 min) with peer count via Swarm API; Dashboard shows online/offline indicator with connected peer count

**Service States**: `starting` → `syncing` → `watching` (or `paused`)

### Key Components

-   **BackupService** ([porcupin/backend/core/service.go](porcupin/backend/core/service.go)) - Orchestrates automatic sync lifecycle
-   **BackupManager** ([porcupin/backend/core/backup.go](porcupin/backend/core/backup.go)) - Handles actual NFT processing and pinning
-   **Indexer** ([porcupin/backend/indexer/tzkt.go](porcupin/backend/indexer/tzkt.go)) - Fetches NFTs via TZKT REST API + WebSocket
-   **IPFS Node** ([porcupin/backend/ipfs/node.go](porcupin/backend/ipfs/node.go)) - Embeds Kubo with Pin, Verify, Cat, Health methods. Content routing uses a DHT **client** + delegated HTTP routing to the IPNI indexer (`cid.contact`) — see `getRoutingOption` in [porcupin/backend/ipfs/profile_default.go](porcupin/backend/ipfs/profile_default.go) and `applyDelegatedRouters`/`SanitizeDelegatedRouters` in [porcupin/backend/ipfs/routing.go](porcupin/backend/ipfs/routing.go). Configurable via `ipfs.delegated_routers` (default `["auto"]`; passed through `ipfs.WithDelegatedRouters(...)` at every `NewNode` call site). IPNI routing is required because most NFT content (Versum, Emprops, nft.storage/web3.storage/Filecoin) advertises providers to IPNI but NOT the Amino DHT; a DHT-only node fails to find them and pins time out.
-   **Logging** ([porcupin/backend/logging/](porcupin/backend/logging/)) - Structured slog with ring buffer, daily rotating log files, multi-handler fan-out, and crash report writer

### Key Patterns

**Wails Bindings**: Go methods on `App` struct in [porcupin/app.go](porcupin/app.go) are auto-exposed to frontend. Add new features by:

1. Adding method to `App` struct
2. Running `wails dev` to regenerate `frontend/wailsjs/go/main/App.{js,d.ts}`

**Asset Status Flow**: `pending` → `pinned` | `failed` | `failed_unavailable` | `skipped`

-   Status constants defined in [porcupin/backend/db/db.go](porcupin/backend/db/db.go)
-   `failed_unavailable` indicates timeout (content not on IPFS network)
-   `skipped` indicates non-IPFS URIs (HTTP/HTTPS) that cannot be pinned

**Concurrency**: `BackupManager.workers` channel implements semaphore limiting concurrent pins (default: 5)

## Development Commands

```bash
cd porcupin

# Development with hot reload
wails dev

# Production build
wails build

# Regenerate Wails JS bindings after Go changes
wails generate module
```

## Database

Uses GORM with SQLite. Schema auto-migrates on startup. Models in [porcupin/backend/db/db.go](porcupin/backend/db/db.go):

-   `Wallet` - Tracked Tezos addresses
-   `NFT` - Token metadata with `artifact_uri`, `thumbnail_uri`
-   `Asset` - Individual IPFS URIs to pin with status tracking

## Configuration

YAML config loaded from `~/.porcupin/config.yaml`. Defaults in [porcupin/backend/config/config.go](porcupin/backend/config/config.go):

-   `ipfs.pin_timeout`: 2 minutes
-   `ipfs.max_file_size`: 5GB
-   `backup.max_concurrency`: 5 workers
-   `backup.min_free_disk_space_gb`: 5GB

## External Dependencies

-   **TZKT API** (`api.tzkt.io`): Tezos blockchain indexer - uses `/v1/tokens/balances` and `/v1/tokens` endpoints (WebSocket support in progress)
-   **Kubo/IPFS**: Embedded node, repo at `~/.porcupin/ipfs`. Provider discovery via DHT + IPNI (`cid.contact`); configured with `ipfs.delegated_routers` (default `["auto"]`) or overridden for one run with the `IPFS_HTTP_ROUTERS` env var.
-   **dipdup-net/go-lib**: TZKT Go client with WebSocket support

## Upcoming Features

-   **Docker deployment**: Container support planned for headless server mode

## Logging & Crash Recovery

-   **Structured logging** via `slog` with fan-out to stderr, in-memory ring buffer (1000 entries), and daily rotating log files (`~/.porcupin/logs/porcupin-YYYY-MM-DD.log`)
-   Standard library `log` output is bridged through `slog`
-   **Crash recovery**: `recover()` wrappers on all goroutines; crash reports written to `~/.porcupin/logs/crash-*.txt` with goroutine stacks and recent log entries
-   **In-app log viewer** in Settings with level filtering (All/Info/Warn/Error) and export functionality
-   **Diagnostic report export** includes version, OS, config summary, asset stats, and recent logs

## Frontend

React + Vite + TypeScript in [porcupin/frontend](porcupin/frontend). Single-page app with tabs:

-   Dashboard (stats polling every 5s, IPFS health indicator with peer count)
-   Wallet management
-   Asset browser with pagination
-   Settings (server connection, storage migration, log viewer with level filtering, export logs/diagnostics, check IPFS connectivity, check for updates)

Call Go via: `import { MethodName } from "../wailsjs/go/main/App"`

## Common Tasks

**Add new API endpoint to App**:

1. Add method to `App` struct in `app.go`
2. Run `wails dev` to regenerate bindings
3. Import from `wailsjs/go/main/App` in React

**Add new database model**:

1. Define struct in `db/db.go` with GORM tags
2. Add to `InitDB()` AutoMigrate call
3. Create helper methods on `Database` struct

**Modify IPFS pinning behavior**:

-   Pin logic in `backupAsset()` and `pinWithRetry()` in `backup.go`
-   Exponential backoff with max 3 retries
