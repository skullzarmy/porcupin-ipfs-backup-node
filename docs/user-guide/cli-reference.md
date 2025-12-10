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

## API Server Mode

The headless server can expose a REST API for remote management by the desktop app or other clients.

### `--serve`

Start the API server for remote access.

```bash
porcupin --serve
```

**First run output:**

```text
API server starting on http://192.168.1.50:8085
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⚠️  NEW API TOKEN GENERATED - SAVE THIS NOW
    Token: prcpn_a7Bx9kL2mN4pQ6rS8tU0vW2xY4zA6bC8dE0fG2hI4j

    This token will not be shown again.
    Store it securely. Use --regenerate-token to create a new one.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**Subsequent runs:** The token is never shown again.

### API Server Options

| Flag                 | Description                                               | Default   |
| -------------------- | --------------------------------------------------------- | --------- |
| `--api-port <port>`  | API server port                                           | `8085`    |
| `--api-bind <addr>`  | API server bind address                                   | `0.0.0.0` |
| `--allow-public`     | Allow connections from public IPs (default: private only) |           |
| `--tls-cert <path>`  | Path to TLS certificate file                              |           |
| `--tls-key <path>`   | Path to TLS private key file                              |           |
| `--regenerate-token` | Regenerate API token and exit                             |           |

### API Token Handling

The API token is **shown only once** when first generated. It cannot be retrieved afterward.

-   **File storage:** `~/.porcupin/.api-token-hash` (stores bcrypt hash, not plaintext)
-   **Environment override:** Set `PORCUPIN_API_TOKEN` to use a specific token
-   **Flag override:** `--api-token <token>` (WARNING: visible in `ps`, prefer env var)

To get a new token:

```bash
porcupin --regenerate-token
```

### API Server Examples

```bash
# Start API server with defaults (port 8085, private IPs only)
porcupin --serve

# Use custom port
porcupin --serve --api-port 9090

# Enable TLS
porcupin --serve --tls-cert /path/to/cert.pem --tls-key /path/to/key.pem

# Allow public IPs (use with caution, requires TLS for security)
porcupin --serve --allow-public --tls-cert cert.pem --tls-key key.pem
```

See [Remote Server Guide](remote-server.md) for complete setup instructions including systemd configuration.

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
──────────────────────────────────────────
Porcupin Stats

  NFTs:          1,234
  Assets:        5,678
  Pinned:        5,500
  Pending:       150
  Failed:        28
  Storage:       45.23 GB

──────────────────────────────────────────
```

### `--version` / `-v`

Show version information with ASCII banner.

```bash
porcupin --version
porcupin -v
```

### `--about`

Show about information including project description and links.

```bash
porcupin --about
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
