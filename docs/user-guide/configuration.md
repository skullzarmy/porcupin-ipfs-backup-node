# Configuration Guide

Porcupin stores its configuration in `~/.porcupin/config.yaml`. The config file is automatically created with default values on first run. You can edit this file directly, use the Settings UI in the desktop app, or use the `porcupin settings` command.

---

## Configuration File Location

| Platform | Path                                                      |
| -------- | --------------------------------------------------------- |
| macOS    | `~/.porcupin/config.yaml`                                 |
| Linux    | `~/.porcupin/config.yaml`                                 |
| Windows  | `C:\Users\<username>\.porcupin\config.yaml`               |
| Docker   | `/home/porcupin/.porcupin/config.yaml` (inside container) |
| Systemd  | `/var/lib/porcupin/config.yaml` (default service path)    |

---

## Full Configuration Reference

```yaml
# IPFS Node Settings
ipfs:
    # Where IPFS stores pinned content
    # Change this to use external storage
    repo_path: ~/.porcupin/ipfs

    # IPFS swarm port for peer-to-peer connections (default: 4001)
    # Change if port 4001 is already in use or blocked by firewall
    swarm_port: 4001

    # Timeout for pinning a single asset (default: 2 minutes)
    # Increase if you have slow internet
    pin_timeout: 2m

    # Maximum file size to pin (default: 5GB)
    # Larger files are skipped
    max_file_size: 5368709120

    # Delegated (HTTP) provider routers, queried in parallel with the DHT to
    # find which peers host a given CID. "auto" expands to the IPNI indexer
    # (cid.contact), which is required to locate most NFT content. Add
    # /routing/v1 URLs to query additional routers. An empty list ([]) uses the
    # DHT only. (Default: ["auto"]. Requires an app restart to take effect.)
    delegated_routers:
        - auto

# Backup Settings
backup:
    # Number of simultaneous downloads (default: 5)
    # Lower this on Raspberry Pi or slow connections
    max_concurrency: 5

    # Stop syncing if free disk space drops below this (GB)
    min_free_disk_space_gb: 5

    # Maximum storage to use (0 = unlimited)
    # Porcupin will pause syncing when this is reached
    max_storage_gb: 0

    # Warn when storage reaches this percentage of max
    storage_warning_pct: 80

    # Default sync settings for new wallets
    sync_owned: true # Sync NFTs you own
    sync_created: true # Sync NFTs you created

# TZKT API Settings
tzkt:
    # Tezos indexer API (usually don't change this)
    base_url: https://api.tzkt.io
```

---

## Common Configuration Scenarios

### Limit Storage Usage

If you have limited disk space:

```yaml
backup:
    max_storage_gb: 100 # Stop at 100GB
    min_free_disk_space_gb: 10 # Keep 10GB free
```

### Use External Storage (macOS/Linux)

Move IPFS data to an external drive:

```yaml
ipfs:
    # macOS external drive
    repo_path: /Volumes/MyExternalDrive/porcupin-ipfs

    # Linux external drive (mounted)
    # repo_path: /mnt/external/porcupin-ipfs
```

**Important:** The drive must be mounted before starting Porcupin.

### Use NAS Storage

For network-attached storage:

```yaml
ipfs:
    # macOS SMB mount
    repo_path: /Volumes/nas-share/porcupin-ipfs

    # Linux NFS mount
    # repo_path: /mnt/nas/porcupin-ipfs
```

**Warning:** Network storage is slower and may cause timeouts. Increase `pin_timeout`:

```yaml
ipfs:
    repo_path: /Volumes/nas-share/porcupin-ipfs
    pin_timeout: 5m # 5 minutes for slow network
```

### Raspberry Pi Optimization

For Raspberry Pi Zero 2 W and other low-memory devices, specific tuning is required.

👉 **See [Minimal Hardware Optimization](installation.md#minimal-hardware-raspberry-pi-zero-2-w) in the Installation Guide for the complete setup.**

### Slow Internet Connection

If downloads frequently timeout:

```yaml
ipfs:
    pin_timeout: 5m # 5 minutes

backup:
    max_concurrency: 2 # Fewer parallel downloads
```

### Only Sync Owned NFTs (Not Created)

If you create many NFTs but only want to back up what you own:

```yaml
backup:
    sync_owned: true
    sync_created: false
```

---

## Content Routing & Provider Discovery

Before Porcupin can pin an asset, it must **find a peer that hosts that content**. Porcupin's embedded IPFS node discovers hosts using two systems at once:

1. **The Amino DHT** (peer-to-peer distributed hash table) — in low-resource _client_ mode.
2. **The IPNI indexer** (`cid.contact`) via delegated HTTP routing — resolved automatically through Kubo AutoConf.

Both are queried in parallel. This dual approach is important: a large amount of NFT content — including **Versum, Emprops, and anything stored via nft.storage / web3.storage / Filecoin** — advertises its providers to the **IPNI indexer only**, not the DHT. A DHT-only node cannot find that content and would report it as "Failed (Unavailable)" even though it is widely available. (This was the cause of large "max retries exceeded" batches before v1.0.4.)

No configuration is required — delegated IPNI routing is enabled by default (`delegated_routers: ["auto"]`), and Porcupin applies it on every start, so even repositories created by older versions get IPNI discovery automatically after upgrading.

### Adding custom provider endpoints (advanced)

You can query additional `/routing/v1` delegated routers — for example a self-hosted [someguy](https://github.com/ipfs/someguy) instance or an alternative indexer — for redundancy, private networks, or self-hosted setups.

**Recommended: the config file.** Add endpoints to the `delegated_routers` list. Keep `auto` to retain IPNI (cid.contact):

```yaml
ipfs:
    delegated_routers:
        - auto # IPNI indexer (cid.contact) — keep this for normal content
        - https://my-router.example/routing/v1
```

Or from the command line:

```bash
porcupin settings ipfs.delegated_routers "auto,https://my-router.example/routing/v1"
```

Or in the **desktop app**: Settings → IPFS Network → Content Provider Routers (one endpoint per line).

Entries must be `auto` or an absolute `http(s)` `/routing/v1` URL; invalid entries are rejected (UI/CLI) or ignored with a warning (config file). Setting an empty list disables delegated routing entirely (DHT only). **Changes take effect after an app restart.**

**Environment override.** Setting the `IPFS_HTTP_ROUTERS` environment variable overrides the config for the current process. Note it **replaces** the list rather than adding to it, so include `cid.contact` explicitly if you still want IPNI:

```bash
export IPFS_HTTP_ROUTERS="https://cid.contact https://my-router.example/routing/v1"
```

None of this is needed for normal use — `cid.contact` (IPNI) already aggregates the major hosting providers.

---

## Migrating Storage Location

### Using the Desktop App

1. Go to **Settings** → **Storage**
2. Click **Change Location**
3. Select the new location
4. Wait for migration to complete

### Using Command Line

The headless version doesn't support migration yet. Manual steps:

1. Stop Porcupin
2. Copy `~/.porcupin/ipfs` to new location
3. Update repo path: `porcupin settings ipfs.repo_path /new/path/ipfs`
4. Start Porcupin

---

## Environment Variables

For Docker or advanced setups, you can use environment variables:

| Variable                  | Description                                                                                                       |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `PORCUPIN_DATA_DIR`       | Override data directory (default: `~/.porcupin`)                                                                  |
| `PORCUPIN_IPFS_PATH`      | Override IPFS repo path                                                                                           |
| `PORCUPIN_MAX_STORAGE_GB` | Override max storage limit                                                                                        |
| `PORCUPIN_API_TOKEN`      | Set API token for remote server mode                                                                              |
| `IPFS_HTTP_ROUTERS`       | Override delegated `/routing/v1` provider endpoints (see [Content Routing](#content-routing--provider-discovery)) |

### API Token via Environment

When running in server mode (`--serve`), you can set the API token via environment variable instead of using the auto-generated token:

```bash
export PORCUPIN_API_TOKEN="prcpn_your_secure_token_here"
porcupin --serve
```

This is the recommended approach for Docker and systemd deployments because:

- The token isn't visible in `ps` output (unlike `--api-token` flag)
- You can rotate the token without regenerating and restarting

**Note:** When `PORCUPIN_API_TOKEN` is set, it takes precedence over the auto-generated token file.

### Example Docker Usage

```bash
docker run -d \
  -e PORCUPIN_MAX_STORAGE_GB=50 \
  -e PORCUPIN_API_TOKEN=prcpn_your_token \
  -p 8085:8085 \
  -v /mnt/data:/home/porcupin/.porcupin \
  ghcr.io/skullzarmy/porcupin-ipfs-backup-node:latest --serve
```

---

## Data Directory Structure

```text
~/.porcupin/
├── config.yaml        # Configuration file
├── porcupin.db        # SQLite database (wallets, NFTs, asset status)
├── .api-token-hash    # API token hash (only when using --serve)
├── logs/              # Log files and crash reports
│   ├── porcupin-YYYY-MM-DD.log  # Daily rolling log files
│   └── crash-*.txt              # Crash reports (on panic)
└── ipfs/              # IPFS repository
    ├── blocks/        # Pinned content (this is the big folder)
    ├── datastore/     # IPFS internal data
    └── config         # IPFS node configuration
```

---

## Managing Settings from the CLI

The `porcupin settings` command lets you view and modify configuration without manually editing the YAML file:

```bash
# List all settings
porcupin settings list

# Show config file path
porcupin settings location

# Get a setting
porcupin settings backup.max_concurrency

# Set a setting
porcupin settings backup.max_concurrency 2
```

Dashes and underscores are interchangeable: `backup.max-concurrency` and `backup.max_concurrency` both work.

See the [CLI Reference](cli-reference.md#settings-commands) for full documentation.

---

## Resetting Configuration

### Reset to Defaults

Delete the config file and restart:

```bash
rm ~/.porcupin/config.yaml
# Restart Porcupin - a new config with defaults will be created automatically
```

### Full Reset (Delete Everything)

**Warning:** This deletes all pinned content!

```bash
rm -rf ~/.porcupin
```

---

## Next Steps

- **[Troubleshooting](troubleshooting.md)** - Common issues
- **[FAQ](faq.md)** - Frequently asked questions
