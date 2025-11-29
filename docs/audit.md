# Adversarial Audit - Porcupin

## 1. IPFS Availability & Performance
-   **Risk**: **Content Already Lost**. If the original host is already offline, `ipfs.Pin()` will hang indefinitely or timeout. We cannot "backup" what is effectively gone.
    -   *Mitigation*: Implement strict timeouts (e.g., 2 minutes) for pinning operations. Mark these as `FAILED_UNAVAILABLE` rather than just generic `FAILED` to inform the user.
-   **Risk**: **Huge Files**. Some NFTs are 4K videos or 3D models (1GB+). Pinning these on a Raspberry Pi could exhaust RAM or crash the node.
    -   *Mitigation*: Stream the download directly to disk (filestore) rather than buffering in RAM. Implement a `MaxFileSize` config option (default e.g., 5GB).
-   **Risk**: **Bandwidth Saturation**. A fresh sync of 10,000 NFTs could saturate the user's home internet connection.
    -   *Mitigation*: Implement a global rate limiter (e.g., "Max 5MB/s") and a "Pause/Resume" feature in the UI.

## 2. Tezos & Indexer Reliability
-   **Risk**: **Malicious/Malformed Metadata**. A user could be air-dropped a token with a 100MB metadata JSON or recursive loops.
    -   *Mitigation*: Enforce a size limit on the Metadata JSON itself (e.g., 1MB). Validate the JSON schema before parsing.
-   **Risk**: **TZKT Rate Limits**. Aggressive syncing (especially with the worker pool) could trigger TZKT's 429 Too Many Requests.
    -   *Mitigation*: Respect the `Retry-After` header. Use a shared rate limiter for all TZKT API calls.
-   **Risk**: **Chain Reorgs**. While rare on Tezos, a reorg could invalidate a "Created" event.
    -   *Mitigation*: Wait for 2-3 confirmations before processing a "Created" event. (Less critical for backup, but good practice).

## 3. System & Security
-   **Risk**: **Disk Space Exhaustion**. The node could fill the disk, causing the OS to crash.
    -   *Mitigation*: Monitor free disk space. Pause operations if free space < 5GB.
-   **Risk**: **Open Web Dashboard**. If the user runs the Docker container on a VPS without a firewall, the dashboard (and wallet management) could be exposed to the public internet.
    -   *Mitigation*: The Docker container should bind to `127.0.0.1` by default. Add Basic Auth support for the web dashboard.
-   **Risk**: **Wails/WebView Security**. If the dashboard renders the NFT content (HTML/SVG) directly, it could be vulnerable to XSS.
    -   *Mitigation*: **NEVER** render the NFT content in the dashboard. Only show the thumbnail. If the thumbnail is SVG, sanitize it or render it in a sandboxed iframe.

## 4. Architecture Flaws
-   **Critique**: The "Worker Pool" is good, but if the app crashes, do we lose the queue state?
    -   *Defense*: The queue is backed by the SQLite database (`status='pending'`). On restart, we simply query for pending items. This is robust.
-   **Critique**: Using `kubo` libraries increases binary size significantly (~50MB+).
    -   *Defense*: Accepted trade-off for the "single binary" requirement.

## Conclusion
The plan is solid, but **Resource Management** (Disk/RAM/Bandwidth) and **Security** (XSS via SVG) are the biggest risks. We must implement strict limits and sanitization.
