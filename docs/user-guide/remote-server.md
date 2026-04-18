# Remote Server Guide

Run Porcupin on a dedicated server (NAS, Raspberry Pi, home server) and manage it remotely from the desktop app.

---

## Overview

The Porcupin headless server can expose a REST API that the desktop GUI connects to over your local network. This allows you to:

-   Run the IPFS node on always-on hardware (NAS, Raspberry Pi, VPS)
-   Manage it from any device on your LAN
-   Keep your NFT backups running 24/7 without a desktop app open

---

## Quick Start

### 1. Start the Server

On your server (Raspberry Pi, NAS, etc.):

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

**⚠️ Save this token immediately!** It cannot be retrieved later.

### 2. Connect from Desktop App

1. Open the Porcupin desktop app
2. Go to **Settings**
3. Under **Server Connection**, click **Add Connection**
4. Enter:
    - **Label:** (e.g., "Home Server")
    - **Server Address:** `192.168.1.50:8085`
    - **API Token:** (paste the token from step 1)
5. Click **Connect**

The app will now manage your remote server.

---

## API Token Security

### Token Shown Once Only

The API token is displayed **only once** when first generated. After that:

-   The plaintext token is **never stored** on the server
-   Only a bcrypt hash is saved to `~/.porcupin/.api-token-hash`
-   There is no way to retrieve the original token

If you lose your token, you must regenerate it:

```bash
porcupin --regenerate-token
```

This creates a new token and invalidates the old one.

### Token Sources (Priority Order)

1. **`--api-token` flag** — ⚠️ Visible in `ps`, avoid if possible
2. **`PORCUPIN_API_TOKEN` env var** — Recommended for Docker/scripts
3. **Token file** — Auto-generated on first `--serve` run

### Example: Using Environment Variable

```bash
export PORCUPIN_API_TOKEN="prcpn_your_token_here"
porcupin --serve
```

---

## systemd Service Configuration

For production deployments, run Porcupin as a systemd service.

### 1. Create Service File

```bash
sudo nano /etc/systemd/system/porcupin.service
```

```ini
[Unit]
Description=Porcupin NFT Backup Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=porcupin
Group=porcupin
ExecStart=/usr/local/bin/porcupin --serve --data /var/lib/porcupin
Restart=always
RestartSec=10

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/porcupin
PrivateTmp=yes

# Environment file for API token (optional)
EnvironmentFile=-/etc/porcupin/env

[Install]
WantedBy=multi-user.target
```

### 2. Create User and Directories

```bash
# Create user
sudo useradd -r -s /usr/sbin/nologin porcupin

# Create data directory
sudo mkdir -p /var/lib/porcupin
sudo chown porcupin:porcupin /var/lib/porcupin
sudo chmod 700 /var/lib/porcupin

# Create config directory (optional, for env file)
sudo mkdir -p /etc/porcupin
```

### 3. First Run (Generate Token)

Run manually first to generate the API token:

```bash
sudo -u porcupin porcupin --serve --data /var/lib/porcupin
```

**Save the token immediately**, then stop with Ctrl+C.

### 4. Enable and Start Service

```bash
sudo systemctl daemon-reload
sudo systemctl enable porcupin
sudo systemctl start porcupin
```

### 5. Check Status

```bash
sudo systemctl status porcupin
sudo journalctl -u porcupin -f
```

---

## Network Configuration

### Default: Private IPs Only

By default, the API server only accepts connections from private IP ranges (LAN):

-   `192.168.0.0/16` (most home networks)
-   `10.0.0.0/8` (some corporate networks)
-   `172.16.0.0/12` (Docker, etc.)
-   `127.0.0.1` (localhost)

Connections from public IPs are rejected with `403 Forbidden`.

### Public Access & Internet Exposure

If you need to access your server from outside your local network (e.g., over the internet), see the dedicated guide:

👉 **[Advanced: Internet Exposure](advanced-exposure.md)**

This guide covers:
-   Risk assessment
-   Enabling public access (`--allow-public`)
-   Configuring TLS (Required for secure access)
-   Router and firewall setup

---

## mDNS Service Discovery

Porcupin advertises itself on the local network using mDNS (Bonjour/Avahi). The desktop app can automatically discover servers without manual IP entry.

### How It Works

1. Server broadcasts `_porcupin._tcp` service on startup
2. Desktop app listens for mDNS announcements
3. Discovered servers appear in Settings → Server Connection

### Requirements

-   **macOS:** Works out of the box (Bonjour)
-   **Linux:** Requires Avahi daemon (`sudo apt install avahi-daemon`)
-   **Windows:** Requires Bonjour Print Services or iTunes installed

### Disable mDNS

If you don't want the server to broadcast its presence:

```bash
porcupin --serve --no-mdns
```

---

## Troubleshooting

### "Connection refused"

1. Check the server is running: `sudo systemctl status porcupin`
2. Check firewall: `sudo ufw allow 8085/tcp`
3. Check the server is listening: `ss -tlnp | grep 8085`

### "403 Forbidden"

You're connecting from a public IP but `--allow-public` is not set. Either:

-   Connect from your LAN
-   Add `--allow-public` (with TLS enabled)

### "401 Unauthorized"

Invalid or missing API token. Verify:

```bash
curl -H "Authorization: Bearer YOUR_TOKEN" http://server:8085/api/v1/health
```

### Lost API Token

Regenerate the token:

```bash
# If running manually:
porcupin --regenerate-token

# If running as systemd service:
sudo systemctl stop porcupin
sudo -u porcupin porcupin --data /var/lib/porcupin --regenerate-token
# Save the new token, then restart:
sudo systemctl start porcupin
```

### Server Not Discovered (mDNS)

1. Ensure client and server are on the same subnet
2. Linux: Check Avahi is running: `systemctl status avahi-daemon`
3. Try manual connection instead

---

## API Reference

The REST API is documented in the source code. Key endpoints:

| Endpoint               | Description                                               |
| ---------------------- | --------------------------------------------------------- |
| `GET /api/v1/health`   | Health check (no auth) — returns `is_online` and `peer_count` |
| `GET /api/v1/status`   | Service status                                            |
| `GET /api/v1/stats`    | Asset statistics                                          |
| `GET /api/v1/wallets`  | List wallets                                              |
| `POST /api/v1/wallets` | Add wallet                                                |
| `POST /api/v1/sync`    | Trigger sync                                              |

### Health Endpoint Response

The `/api/v1/health` endpoint returns IPFS connectivity information:

```json
{
  "status": "ok",
  "version": "1.0.0",
  "timestamp": "2026-03-18T12:00:00Z",
  "is_online": true,
  "peer_count": 42
}
```

- `is_online` — whether the IPFS node has connected peers
- `peer_count` — number of currently connected IPFS swarm peers

All endpoints except `/health` require:

```text
Authorization: Bearer <token>
```

---

## See Also

-   [CLI Reference](cli-reference.md) — All command-line options
-   [Configuration](configuration.md) — Config file options
-   [Installation](installation.md) — Installing Porcupin
