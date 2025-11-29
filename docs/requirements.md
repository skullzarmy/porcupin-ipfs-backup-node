# Porcupin Tezos Backup Node - Requirements Document

## 1. Introduction
Porcupin is a modern, optimized native application and self-hosted solution designed to preserve Tezos NFT history. It acts as a specialized backup node that connects to the Tezos blockchain, identifies NFTs associated with specific wallets (both owned and created), and ensures their content (metadata, images, assets) is pinned to IPFS, preventing data loss due to cache eviction or host disappearance.

## 2. Problem Statement
Collectors and creators on Tezos face the risk of losing their NFT assets (images, videos, metadata) if the original IPFS gateways or pinning services stop hosting the files. While the blockchain record remains, the actual media can be lost. Existing solutions (local backup, Internet Archive) do not solve the availability problem *on the IPFS network* itself.

## 3. Core Objectives
- **Preservation**: Ensure NFT assets remain available on the IPFS network via repinning.
- **Availability**: Provide a robust, always-on node (Desktop or Server) to serve these assets.
- **Ease of Use**: Offer a simple "set and forget" experience for users, with advanced options for power users.
- **Performance**: Utilize optimized, low-latency methods (WebSockets, efficient indexing) to track chain state.

## 4. Functional Requirements

### 4.1. Tezos Blockchain Integration
- **Data Source**: TZKT API (Mainnet).
- **Scope & Endpoints**:
    - **Owned NFTs**:
        - **Endpoint**: `/v1/tokens/balances`
        - **Params**: `account={address}`, `token.metadata.artifactUri.null=false`, `limit=1000`
        - **Select**: `token.id,token.contract,token.tokenId,token.metadata`
    - **Created NFTs**:
        - **Endpoint**: `/v1/tokens`
        - **Params**: `firstMinter={address}`, `limit=1000`
        - **Select**: `id,contract,tokenId,metadata`
- **Data Extraction**:
    - **Asset URI**: `metadata.artifactUri`
    - **Thumbnail URI**: `metadata.thumbnailUri`
    - **Formats**: `metadata.formats[].uri`
    - **Raw Metadata URI**: Query `token_metadata` BigMap -> `token_info[""]` (Hex Decoded).
- **Optimization**:
    - Use `lastId` pagination for historical syncs.
    - Listen to `transfers` WebSocket channel for real-time updates.

### 4.2. IPFS Integration
- **Node Type**: Embedded IPFS node (using best-in-class libraries like Kubo/Go-IPFS) or connection to external daemon.
- **Pinning**:
    - Automatically pin metadata JSON.
    - Parse metadata to find asset URIs (artifactUri, displayUri, thumbnailUri).
    - Recursively pin all referenced assets.
- **Resource Limits**:
    - **Pin Timeout**: 2 minutes per asset (mark as `FAILED_UNAVAILABLE` if timeout).
    - **Max File Size**: Configurable (default 5GB).
    - **Bandwidth Limiter**: Configurable max download rate (default unlimited).
- **Garbage Collection**: (Optional/Advanced) Option to unpin assets if an NFT is burned or transferred out (configurable).

### 4.3. Backup Logic
- **Wallets**: Support tracking multiple wallet addresses.
- **Queue System**: Robust queue for processing assets to avoid overwhelming the network or local resources.
- **Retry Mechanism**: Exponential backoff for failed downloads/pins.
- **Status Reporting**: Track sync status (Synced, Pending, Failed, Failed_Unavailable) for each NFT.
- **Metadata Validation**:
    - Enforce 1MB max size for metadata JSON.
    - Schema validation before parsing.
    - Reject malformed/recursive structures.

### 4.4. User Interface
- **Desktop App**: Native look and feel for macOS, Windows, Linux.
- **Web Dashboard**: For the headless/server/Docker version.
- **Features**:
    - Dashboard showing total pinned size, number of NFTs, node status.
    - Wallet management (Add/Remove/Label).
    - Activity log / Console output.
    - Settings (IPFS limits, Storage path, Network bandwidth).

### 4.5. Deployment & Platforms
- **Desktop**: macOS (Universal), Windows (x64/ARM), Linux (Debian/RPM).
- **Raspberry Pi**: Optimized for ARM64 architecture (Raspberry Pi 4/5).
- **Docker**: Self-contained container for cloud/server hosting.

## 5. Non-Functional Requirements

### 5.1. Performance
- **Resource Usage**: Low memory footprint (crucial for Raspberry Pi). Stream downloads to disk.
- **Concurrency**: Parallel processing of downloads/pins (configurable limits).
- **Storage**: Efficient local storage management. Pause operations if free disk < 5GB.
- **Rate Limiting**: Respect TZKT API rate limits (429 errors). Use exponential backoff.

### 5.2. Security
- **Isolation**: Docker container should run as non-root where possible.
- **Encryption**: API keys (if any) or sensitive config should be stored securely.
- **Updates**: Auto-update mechanism for the desktop app.
- **Web Dashboard**: Bind to `127.0.0.1` by default. Support Basic Auth for remote access.
- **XSS Protection**: Never render NFT content in the dashboard. Sanitize SVG thumbnails.

### 5.3. Code Quality
- **Style**: "Leetcode style" optimized solutions for critical paths (e.g., parsing, data structures).
- **Libraries**: Use established, maintained open-source libraries. No "Not Invented Here" syndrome for standard protocols.

## 6. Documentation
- **Wiki**: GitHub Wiki with comprehensive guides.
- **Tutorials**:
    - "Getting Started on Mac/Windows"
    - "Setting up a Raspberry Pi Node"
    - "Deploying to Cloud (AWS/DigitalOcean) with Docker"

