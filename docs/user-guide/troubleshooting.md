# Troubleshooting Guide

Common issues and solutions for Porcupin.

---

## Installation Issues

### macOS: "Porcupin can't be opened because it is from an unidentified developer"

**Solution:** Right-click the app → Open → Click "Open" in the dialog.

Or run in Terminal:

```bash
xattr -cr /Applications/Porcupin.app
```

### macOS: "Porcupin is damaged and can't be opened"

This happens when macOS quarantines the app. Run:

```bash
xattr -cr /Applications/Porcupin.app
```

### Linux: GTK/WebKit errors

Install required dependencies:

**Ubuntu/Debian:**

```bash
sudo apt install libgtk-3-0 libwebkit2gtk-4.0-37
```

**Fedora:**

```bash
sudo dnf install gtk3 webkit2gtk3
```

### Windows: Missing DLL errors

Install the latest [Visual C++ Redistributable](https://learn.microsoft.com/en-us/cpp/windows/latest-supported-vc-redist).

---

## IPFS Node Issues

### "IPFS node won't start" / "Failed to start IPFS node"

**Cause 1: Port 4001 is already in use**

Check what's using the port:

```bash
# macOS/Linux
lsof -i :4001

# Windows
netstat -ano | findstr :4001
```

Stop the other IPFS process or change ports.

**Cause 2: Stale lock file**

If Porcupin crashed, a lock file may remain:

```bash
rm ~/.porcupin/ipfs/repo.lock
```

**Cause 3: Corrupted IPFS repo**

As a last resort, delete and let Porcupin recreate it:

```bash
rm -rf ~/.porcupin/ipfs
# Restart Porcupin
```

**Warning:** This deletes all pinned content.

### "IPFS node takes forever to start"

On first launch, IPFS generates cryptographic keys which can take 30-60 seconds. Subsequent starts are faster.

If it consistently takes long, your disk may be slow. Consider using an SSD.

---

## Sync Issues

### Assets stuck in "Pending"

**Cause 1: Slow internet**

Increase timeout in `~/.porcupin/config.yaml`:

```yaml
ipfs:
    pin_timeout: 5m
```

**Cause 2: Content not available on IPFS**

Some NFT content is no longer on the IPFS network. Porcupin will mark these as "Failed (Unavailable)" after timeout.

**Cause 3: Too many concurrent downloads**

Reduce concurrency:

```yaml
backup:
    max_concurrency: 2
```

### Assets marked "Failed (Unavailable)"

This means the content isn't available anywhere on IPFS. This happens when:

-   The original host stopped pinning
-   No one else has a copy
-   The IPFS gateway is temporarily down

**What you can do:**

1. Wait - Porcupin periodically retries
2. Check if the NFT platform still shows the image
3. If visible on a website, the content may come back

### Sync is very slow

**For large collections (1000+ NFTs):**

-   First sync always takes time
-   Reduce `max_concurrency` to avoid overwhelming your connection
-   Be patient - it's a one-time process

**For ongoing syncs:**

-   Check your internet connection
-   Look at logs for errors

---

## Storage Issues

### "Low disk space" warnings

Porcupin pauses when free space drops below `min_free_disk_space_gb`.

**Solutions:**

1. Free up disk space
2. Move IPFS storage to external drive (see [Configuration](configuration.md))
3. Set a storage limit: `max_storage_gb: 100`

### Disk space not freed after clearing data

**macOS:** Time Machine snapshots may be holding deleted data. Either:

-   Wait for macOS to thin snapshots automatically
-   Manually thin: `sudo tmutil thinlocalsnapshots / 9999999999999 1`

**All platforms:** IPFS garbage collection runs after clearing. This can take time for large repos.

### "Storage migration failed"

**Cause 1: Destination not writable**

```bash
# Check permissions
ls -la /path/to/destination
# Fix if needed
chmod 755 /path/to/destination
```

**Cause 2: Not enough space at destination**

Ensure destination has at least as much free space as current IPFS repo size.

**Cause 3: Network drive disconnected**

For NAS/network storage, ensure it's mounted before migrating.

---

## Database Issues

### "Failed to open database"

The SQLite database may be corrupted. Try:

1. Stop Porcupin
2. Backup the database: `cp ~/.porcupin/porcupin.db ~/.porcupin/porcupin.db.backup`
3. Delete and let Porcupin recreate: `rm ~/.porcupin/porcupin.db`
4. Restart Porcupin

**Note:** You'll need to re-add wallets, but IPFS pins are preserved.

---

## Network Issues

### "Failed to fetch from TZKT API"

**Cause 1: TZKT is down**

Check [TZKT status](https://api.tzkt.io/) - if it's down, wait.

**Cause 2: Rate limited**

If you're syncing many wallets rapidly, TZKT may rate limit you. Porcupin handles this automatically with backoff.

**Cause 3: Firewall blocking**

Ensure outbound HTTPS (port 443) is allowed.

### IPFS not connecting to peers

Check if port 4001 is open:

-   **Router:** Forward port 4001 TCP/UDP
-   **Firewall:** Allow port 4001

Without peer connections, Porcupin can still pin but won't share with the network.

---

## Platform-Specific Issues

### Raspberry Pi: Very slow performance

**Cause 1: Using SD card**

SD cards are too slow for IPFS. Use an external SSD:

```yaml
ipfs:
    repo_path: /mnt/ssd/porcupin-ipfs
```

**Cause 2: 32-bit OS**

Use 64-bit Raspberry Pi OS. The 32-bit binary is not provided.

**Cause 3: Not enough RAM**

Raspberry Pi 4 with 2GB may struggle. 4GB+ recommended.

### Docker: Permission denied

If running as non-root:

```bash
# Ensure volume is writable
docker run -d \
  --user $(id -u):$(id -g) \
  -v /path/to/data:/home/porcupin/.porcupin \
  ghcr.io/skullzarmy/porcupin
```

---

## Getting Help

If your issue isn't listed:

1. **Check logs:**

    - Desktop: View → Developer Tools → Console
    - Headless: Check stdout/stderr or `journalctl -u porcupin`
    - Docker: `docker logs porcupin`

2. **Search existing issues:** [GitHub Issues](https://github.com/skullzarmy/porcupin-ipfs-backup-node/issues)

3. **Open a new issue** with:
    - Your platform (macOS/Windows/Linux/Docker)
    - Porcupin version (`porcupin --version`)
    - Relevant log output
    - Steps to reproduce
