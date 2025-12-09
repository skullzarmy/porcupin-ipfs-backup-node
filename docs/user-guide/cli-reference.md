# CLI Reference

Complete reference for the Porcupin headless server command-line interface.

---

## Synopsis

```bash
porcupin [options]
```

Running `porcupin` without any one-off command flags starts the daemon, which:

1. Starts the embedded IPFS node
2. Syncs all configured wallets
3. Watches for new NFTs
4. Pins assets to IPFS

---

## Global Options

| Flag              | Description                               | Default              |
| ----------------- | ----------------------------------------- | -------------------- |
| `--data <path>`   | Data directory for database and IPFS repo | `~/.porcupin`        |
| `--config <path>` | Path to config file                       | `<data>/config.yaml` |
| `--version`       | Show version and exit                     |                      |

---

## One-Off Commands

These commands execute immediately and exit (they don't start the daemon).

### `--add-wallet <address>`

Add a Tezos wallet to track.

```bash
porcupin --add-wallet tz1YourWalletAddress
```

**Note:** If running as a systemd service, you must restart the service after adding a wallet for it to start syncing:

```bash
sudo -u porcupin porcupin --data /var/lib/porcupin --add-wallet tz1YourWallet
sudo systemctl restart porcupin
```

### `--remove-wallet <address>`

Remove a wallet from tracking.

```bash
porcupin --remove-wallet tz1YourWalletAddress
```

**Note:** This removes the wallet but does not unpin its assets from IPFS.

### `--list-wallets`

List all tracked wallets.

```bash
porcupin --list-wallets
```

Output:

```
Tracked wallets:
  tz1abc123... - Main Wallet
  tz2def456... - (no alias)
```

### `--stats`

Show current backup statistics.

```bash
porcupin --stats
```

Output:

```
Porcupin Stats:
  Total NFTs:     1,234
  Total Assets:   5,678
  Pinned:         5,500
  Pending:        150
  Failed:         28
  Storage Used:   45.23 GB
```

### `--version`

Show version information.

```bash
porcupin --version
```

Output:

```
porcupin <VERSION>
```

### `--retry-pending`

Process all assets stuck in "pending" status.

```bash
porcupin --retry-pending
```

This is useful when:

-   A previous sync was interrupted (killed, crashed, etc.)
-   Assets were created but never pinned
-   You want to resume incomplete work

**Note:** This starts the IPFS node, processes pending assets, then exits. It does not start the full daemon.

Example with systemd:

```bash
sudo -u porcupin porcupin --data /var/lib/porcupin --retry-pending
```

---

## Usage with systemd

When running Porcupin as a systemd service with a dedicated user, you must:

1. Use `--data` to specify the data directory
2. Run commands as the `porcupin` user

### Examples

```bash
# Add a wallet
sudo -u porcupin porcupin --data /var/lib/porcupin --add-wallet tz1YourWallet

# List wallets
sudo -u porcupin porcupin --data /var/lib/porcupin --list-wallets

# Check stats
sudo -u porcupin porcupin --data /var/lib/porcupin --stats

# Restart service after adding wallet
sudo systemctl restart porcupin

# View logs
sudo journalctl -u porcupin -f
```

---

## Data Directory Structure

The `--data` directory contains:

```
~/.porcupin/           # or /var/lib/porcupin for systemd
├── config.yaml        # Configuration file
├── porcupin.db        # SQLite database (wallets, NFTs, assets)
└── ipfs/              # IPFS repository
    ├── config
    ├── datastore/
    └── blocks/        # Pinned content
```

---

## Exit Codes

| Code | Meaning                          |
| ---- | -------------------------------- |
| 0    | Success                          |
| 1    | Error (check stderr for details) |

---

## Signals

When running as a daemon:

| Signal            | Action            |
| ----------------- | ----------------- |
| `SIGINT` (Ctrl+C) | Graceful shutdown |
| `SIGTERM`         | Graceful shutdown |

---

## See Also

-   [Installation Guide](installation.md) - Installing Porcupin
-   [Quick Start Guide](quickstart.md) - Getting started
-   [Configuration](configuration.md) - Config file options
-   [Troubleshooting](troubleshooting.md) - Common issues
