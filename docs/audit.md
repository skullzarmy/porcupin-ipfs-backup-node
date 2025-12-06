# Adversarial Audit - Porcupin

This document outlines potential security and reliability risks identified during development, along with the mitigations implemented to address them.

---

## 1. IPFS Availability & Performance

### Content Already Lost

**Risk**: If the original IPFS host is offline, `ipfs.Pin()` will hang indefinitely or timeout. Content that is effectively gone cannot be backed up.

**Mitigation**: Implemented strict 2-minute timeouts for pinning operations. Assets that timeout are marked as `FAILED_UNAVAILABLE` (distinct from generic `FAILED`) to inform users that the content is no longer available on the IPFS network.

### Huge Files

**Risk**: Some NFTs are 4K videos or 3D models (1GB+). Pinning these on resource-constrained devices could exhaust RAM or crash the node.

**Mitigation**: Downloads stream directly to disk via IPFS filestore rather than buffering in RAM. A configurable `MaxFileSize` option (default: 5GB) prevents excessively large files from being pinned.

### Bandwidth Saturation

**Risk**: A fresh sync of 10,000+ NFTs could saturate the user's home internet connection.

**Mitigation**: Implemented configurable concurrent worker limits (default: 5) and a "Pause/Resume" feature in the UI to give users control over sync timing.

---

## 2. Tezos & Indexer Reliability

### Malicious/Malformed Metadata

**Risk**: A user could be airdropped a token with a 100MB metadata JSON or malformed data designed to crash the parser.

**Mitigation**: Enforced size limits on metadata JSON. Schema validation occurs before parsing to reject malformed data.

### TZKT Rate Limits

**Risk**: Aggressive syncing could trigger TZKT's 429 Too Many Requests responses.

**Mitigation**: Implemented exponential backoff with retry logic. All TZKT API calls respect rate limits and `Retry-After` headers.

### Chain Reorganizations

**Risk**: While rare on Tezos, a reorg could invalidate a recently processed event.

**Mitigation**: For backup purposes, this is low-risk since we're preserving content, not tracking ownership. The incremental sync mechanism naturally handles any corrections on subsequent syncs.

---

## 3. System & Security

### Disk Space Exhaustion

**Risk**: The IPFS node could fill the disk, causing the OS to become unstable or crash.

**Mitigation**: Implemented free disk space monitoring. Operations pause automatically when free space falls below a configurable threshold (default: 5GB).

### Open Web Dashboard

**Risk**: Running the Docker container on a VPS without proper firewall configuration could expose the dashboard and wallet management to the public internet.

**Mitigation**: The Docker container binds to `127.0.0.1` by default, preventing external access. Users must explicitly configure port forwarding if external access is desired.

### XSS via NFT Content

**Risk**: If the dashboard renders NFT content (HTML/SVG) directly, it could be vulnerable to cross-site scripting attacks.

**Mitigation**: The dashboard never renders raw NFT content. Only thumbnails are displayed, and these are served through safe image handling rather than direct HTML rendering.

---

## 4. Architecture & Resilience

### Queue State Persistence

**Concern**: If the app crashes during sync, is the queue state lost?

**Resolution**: The work queue is backed by the SQLite database (`status='pending'`). On restart, the application queries for pending items and resumes where it left off. This provides crash-resilient operation.

### Binary Size

**Concern**: Embedding Kubo libraries increases binary size significantly (~50MB+).

**Resolution**: Accepted trade-off for the "single binary" requirement. The embedded node eliminates external dependencies and simplifies deployment across all supported platforms.

---

## Summary

The primary risks addressed are **resource management** (disk/RAM/bandwidth exhaustion) and **security** (XSS, network exposure). All identified risks have corresponding mitigations implemented in the codebase.
