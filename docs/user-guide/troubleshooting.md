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

If Porcupin crashed, a lock file may remain. The app will now display a dialog with the specific error and the lock file path. In most cases, stale locks from crashed processes are automatically released by the OS (via flock semantics), so simply restarting the app should work. If the lock persists:

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

**Cause 2: Content couldn't be located in time**

Before Porcupin can pin a file, it has to find a peer on the IPFS network that is hosting it. If no provider is found within the pin timeout, the asset is marked "Failed (Unavailable)". This usually means one of:

- The content genuinely has no online host anymore, **or**
- The provider only advertises itself through the IPNI indexer (`cid.contact`) rather than the DHT. Porcupin queries **both** the DHT and IPNI as of v1.0.4+, which resolves the large batches of "max retries exceeded" failures seen on Versum, Emprops, and other content stored via nft.storage / web3.storage / Filecoin. If you saw many such failures on an older version, simply update and let the retry worker run — no re-import needed.

**Cause 3: Too many concurrent downloads**

Reduce concurrency:

```yaml
backup:
    max_concurrency: 2
```

### Assets marked "Failed (Unavailable)"

This means Porcupin couldn't locate and retrieve a provider for the content within the timeout. It does **not** always mean the content is gone. Common causes:

- The original host stopped pinning and no one else has a copy (genuinely lost)
- The content is available but slow to discover/fetch (transient)
- You are on a version older than v1.0.4, which only queried the DHT and missed content advertised via the IPNI indexer

**What you can do:**

1. **Update Porcupin** if you're on an older version — v1.0.4+ queries both the DHT and IPNI (`cid.contact`), which recovers most previously "unavailable" assets on the next retry.
2. Wait — Porcupin periodically retries failed assets automatically.
3. Check if the NFT platform still shows the image. If a public gateway (e.g. `https://ipfs.io/ipfs/<cid>`) or `https://cid.contact/cid/<cid>` shows providers, the content is on the network and should pin.
4. If your connection is slow, raise `ipfs.pin_timeout` in `~/.porcupin/config.yaml`.

### Sync is very slow

**For large collections (1000+ NFTs):**

- First sync always takes time
- Reduce `max_concurrency` to avoid overwhelming your connection
- Be patient - it's a one-time process

**For ongoing syncs:**

- Check your internet connection
- Look at logs for errors

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

- Wait for macOS to thin snapshots automatically
- Manually thin: `sudo tmutil thinlocalsnapshots / 9999999999999 1`

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

### macOS: "Connection failed" when connecting to remote server

**Symptom:** Remote server connection works when app is launched from Terminal, but fails when launched from Finder/Applications.

**Cause:** Missing network entitlements. macOS sandboxes apps launched from Finder and requires explicit entitlements for network access.

**Solution for users:** This is a bug in the app build. Please report it. As a workaround, launch from Terminal:

```bash
/Applications/Porcupin.app/Contents/MacOS/porcupin
```

**Solution for developers:** Ensure `build/darwin/entitlements.plist` includes:

```xml
<key>com.apple.security.network.client</key>
<true/>
<key>com.apple.security.network.server</key>
<true/>
```

Then rebuild with `wails build`.

### Windows: Remote connections not working

**Cause:** Windows Firewall blocked the app on first launch.

**Solution:**

1. Open Windows Security → Firewall & network protection → Allow an app through firewall
2. Find Porcupin and ensure both Private and Public are checked
3. Or when the firewall prompt appears on first launch, click "Allow access"

### "Failed to fetch from TZKT API"

**Cause 1: TZKT is down**

Check [TZKT status](https://api.tzkt.io/) - if it's down, wait.

**Cause 2: Rate limited**

If you're syncing many wallets rapidly, TZKT may rate limit you. Porcupin handles this automatically with backoff.

**Cause 3: Firewall blocking**

Ensure outbound HTTPS (port 443) is allowed.

### IPFS not connecting to peers

Check if port 4001 is open:

- **Router:** Forward port 4001 TCP/UDP
- **Firewall:** Allow port 4001

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

### Docker: Permission denied (Volume mapping)

If running as non-root:

```bash
# Ensure volume is writable
docker run -d \
  --user $(id -u):$(id -g) \
  -v /path/to/data:/home/porcupin/.porcupin \
  ghcr.io/skullzarmy/porcupin-ipfs-backup-node
```

### Linux: App crashes overnight

Porcupin is designed to run 24/7. On Linux, the system may suspend to RAM after a period of inactivity, even with the screensaver disabled. This can cause the app to crash because the embedded WebKit browser does not handle suspend/resume gracefully.

**To disable system suspend:**

```bash
sudo systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target
```

**To re-enable later:**

```bash
sudo systemctl unmask sleep.target suspend.target hibernate.target hybrid-sleep.target
```

**To verify current state:**

```bash
systemctl status sleep.target suspend.target
```

If the app has crashed, crash reports are saved to `~/.porcupin/logs/crash-*.txt`. You can also export a full diagnostic bundle from **Settings → Logs & Diagnostics → Export Diagnostic Report**.

---

## Uninstalling / Full Reset

If you need to completely remove Porcupin or start fresh, follow the instructions for your platform below.

### What Gets Stored Where

Porcupin stores all user data in a single directory:

| Data              | Location                       |
| ----------------- | ------------------------------ |
| Database          | `~/.porcupin/porcupin.db`      |
| IPFS repo         | `~/.porcupin/ipfs/`            |
| Config            | `~/.porcupin/config.yaml`      |
| Logs              | `~/.porcupin/logs/`            |

> `~` means your **home directory** — e.g., `/home/yourname` on Linux, `/Users/yourname` on macOS, or `C:\Users\yourname` on Windows.

### macOS

```bash
# 1. Quit Porcupin

# 2. Remove the application
rm -rf /Applications/Porcupin.app

# 3. Remove all user data (database, IPFS repo, config, logs)
rm -rf ~/.porcupin
```

### Windows

1. Quit Porcupin
2. Delete the Porcupin folder from where you installed it (e.g., `C:\Program Files\Porcupin\` or your Desktop)
3. Remove all user data — open PowerShell and run:

```powershell
Remove-Item -Recurse -Force "$env:USERPROFILE\.porcupin"
```

Or manually delete the `.porcupin` folder in your user directory (`C:\Users\yourname\.porcupin`).

### Linux

```bash
# 1. Quit Porcupin

# 2. Remove the binary
# If installed to /usr/local/bin:
sudo rm /usr/local/bin/porcupin
# Or delete wherever you placed the binary/AppImage

# 3. Remove all user data (database, IPFS repo, config, logs)
rm -rf ~/.porcupin
```

### Docker

```bash
docker stop porcupin
docker rm porcupin
docker rmi porcupin

# Remove the mounted data volume (if using bind mount):
rm -rf /path/to/your/porcupin-data
```

### Fresh Start (Keep App, Reset Data)

If you just want to start over without removing the app itself:

1. Use **Settings → Danger Zone → Clear All Data** in the app to unpin content and clear the database while keeping your wallets
2. Or manually reset by quitting the app and running:

```bash
rm -rf ~/.porcupin
```

The app will recreate all necessary files and directories on next launch.

---

## Getting Help

If your issue isn't listed:

1. **Check logs:**
    - Desktop: **Settings → Logs & Diagnostics** — view recent logs and export a diagnostic report
    - Log files: `~/.porcupin/logs/porcupin-YYYY-MM-DD.log`
    - Crash reports: `~/.porcupin/logs/crash-*.txt`
    - Headless: Check stdout/stderr or `journalctl -u porcupin`
    - Docker: `docker logs porcupin`

2. **Search existing issues:** [GitHub Issues](https://github.com/skullzarmy/porcupin-ipfs-backup-node/issues)

3. **Open a new issue** with:
    - Your platform (macOS/Windows/Linux/Docker)
    - Porcupin version (`porcupin --version`)
    - Relevant log output
    - Steps to reproduce
