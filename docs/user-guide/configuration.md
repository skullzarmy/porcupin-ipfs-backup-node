# Configuration Guide

Porcupin stores its configuration in `~/.porcupin/config.yaml`. You can edit this file directly or use the Settings UI in the desktop app.

---

## Configuration File Location

| Platform | Path                                                      |
| -------- | --------------------------------------------------------- |
| macOS    | `~/.porcupin/config.yaml`                                 |
| Linux    | `~/.porcupin/config.yaml`                                 |
| Windows  | `C:\Users\<username>\.porcupin\config.yaml`               |
| Docker   | `/home/porcupin/.porcupin/config.yaml` (inside container) |

---

## Full Configuration Reference

```yaml
# IPFS Node Settings
ipfs:
    # Where IPFS stores pinned content
    # Change this to use external storage
    repo_path: ~/.porcupin/ipfs

    # Timeout for pinning a single asset (default: 2 minutes)
    # Increase if you have slow internet
    pin_timeout: 2m

    # Maximum file size to pin (default: 5GB)
    # Larger files are skipped
    max_file_size: 5368709120

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

For Raspberry Pi with limited resources:

```yaml
ipfs:
    # Use external SSD (recommended)
    repo_path: /mnt/ssd/porcupin-ipfs
    pin_timeout: 3m # Allow more time

backup:
    max_concurrency: 2 # Fewer parallel downloads
    min_free_disk_space_gb: 2
```

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
3. Update `repo_path` in config.yaml
4. Start Porcupin

---

## Environment Variables

For Docker or advanced setups, you can use environment variables:

| Variable                  | Description                                      |
| ------------------------- | ------------------------------------------------ |
| `PORCUPIN_DATA_DIR`       | Override data directory (default: `~/.porcupin`) |
| `PORCUPIN_IPFS_PATH`      | Override IPFS repo path                          |
| `PORCUPIN_MAX_STORAGE_GB` | Override max storage limit                       |

Example Docker usage:

```bash
docker run -d \
  -e PORCUPIN_MAX_STORAGE_GB=50 \
  -v /mnt/data:/home/porcupin/.porcupin \
  ghcr.io/skullzarmy/porcupin:latest
```

---

## Data Directory Structure

```
~/.porcupin/
├── config.yaml      # Configuration file
├── porcupin.db      # SQLite database (wallets, NFTs, asset status)
└── ipfs/            # IPFS repository
    ├── blocks/      # Pinned content (this is the big folder)
    ├── datastore/   # IPFS internal data
    └── config       # IPFS node configuration
```

---

## Resetting Configuration

### Reset to Defaults

Delete the config file and restart:

```bash
rm ~/.porcupin/config.yaml
# Restart Porcupin - a new config with defaults will be created
```

### Full Reset (Delete Everything)

**Warning:** This deletes all pinned content!

```bash
rm -rf ~/.porcupin
```

---

## Next Steps

-   **[Troubleshooting](troubleshooting.md)** - Common issues
-   **[FAQ](faq.md)** - Frequently asked questions
