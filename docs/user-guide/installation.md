# Installation Guide

This guide covers installing Porcupin on all supported platforms.

---

## Quick Reference: Which Binary Do I Need?

| Platform                  | Architecture | Download                       | Notes                              |
| ------------------------- | ------------ | ------------------------------ | ---------------------------------- |
| **macOS (Intel)**         | x64          | `porcupin-macos.zip`           | Universal binary works on Intel    |
| **macOS (Apple Silicon)** | ARM64        | `porcupin-macos.zip`           | Universal binary works on M1/M2/M3 |
| **Windows (x64)**         | x64          | `porcupin-windows-amd64.zip`   | Windows 10/11                      |
| **Windows (ARM)**         | ARM64        | `porcupin-windows-arm64.zip`   | Surface Pro X, etc.                |
| **Linux (x64)**           | x64          | `porcupin-linux-amd64.tar.gz`  | Ubuntu/Debian with GUI             |
| **Linux Server (x64)**    | x64          | `porcupin-server-linux-amd64`  | Headless Ubuntu/Debian             |
| **Linux Server (ARM64)**  | ARM64        | `porcupin-server-linux-arm64`  | Headless, Raspberry Pi 4/5         |
| **macOS Server (Intel)**  | x64          | `porcupin-server-darwin-amd64` | Headless macOS Intel               |
| **macOS Server (ARM)**    | ARM64        | `porcupin-server-darwin-arm64` | Headless macOS M1/M2/M3            |
| **Docker**                | Any          | `ghcr.io/skullzarmy/porcupin`  | Any platform with Docker           |

All downloads available at [Releases](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest).

---

## Desktop App (GUI)

### macOS

1. Download `porcupin-macos.zip` from [Releases](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest)
2. Unzip and drag `porcupin.app` to your Applications folder
3. First launch: Right-click → Open (to bypass Gatekeeper)
4. Add your wallet addresses and you're done!

**System Requirements:**

-   macOS 11 (Big Sur) or later
-   4GB RAM minimum
-   10GB+ free disk space (more = more NFTs)

### Windows

1. Download `porcupin-windows-amd64.zip` (or `arm64` for ARM devices) from [Releases](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest)
2. Extract and run `porcupin.exe`
3. Add your wallet addresses and you're done!

**System Requirements:**

-   Windows 10/11 (64-bit)
-   4GB RAM minimum
-   10GB+ free disk space

### Linux (x64)

1. Download `porcupin-linux-amd64.tar.gz` from [Releases](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest)
2. Extract: `tar -xzf porcupin-linux-amd64.tar.gz`
3. Run: `./porcupin`
4. Add your wallet addresses and you're done!

**System Requirements:**

-   Ubuntu 22.04+ / Debian 12+ (or equivalent with WebKit2GTK 4.1)
-   `libwebkit2gtk-4.1` installed (`sudo apt install libwebkit2gtk-4.1-0`)
-   4GB RAM minimum
-   10GB+ free disk space

**Note:** If you prefer a headless server (no GUI), see the [Headless Server](#headless-server-no-gui) section below.

---

## Headless Server (No GUI)

For servers, VPS, or Raspberry Pi where you don't need a graphical interface.

### Ubuntu/Debian Server (x64)

```bash
# Download the binary
wget https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest/download/porcupin-server-linux-amd64

# Make executable
chmod +x porcupin-server-linux-amd64

# Move to a system location (optional)
sudo mv porcupin-server-linux-amd64 /usr/local/bin/porcupin

# Verify it works
porcupin --version
```

### Raspberry Pi (ARM64)

**Prerequisites:** Raspberry Pi 4 or 5 with 64-bit Raspberry Pi OS

```bash
# Download the ARM64 binary
wget https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest/download/porcupin-server-linux-arm64

# Make executable
chmod +x porcupin-server-linux-arm64

# Move to a system location (optional)
sudo mv porcupin-server-linux-arm64 /usr/local/bin/porcupin

# Verify it works
porcupin --version
```

**Tip:** For Raspberry Pi, consider using an external SSD for storage. SD cards are slow and wear out quickly with IPFS.

### Running as a Service (systemd)

Create `/etc/systemd/system/porcupin.service`:

```ini
[Unit]
Description=Porcupin NFT Backup Node
After=network.target

[Service]
Type=simple
User=porcupin
ExecStart=/usr/local/bin/porcupin --data /var/lib/porcupin
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Then:

```bash
# Create a dedicated user and data directory
sudo useradd -r -s /bin/false porcupin
sudo mkdir -p /var/lib/porcupin
sudo chown porcupin:porcupin /var/lib/porcupin

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable porcupin
sudo systemctl start porcupin

# Check status
sudo systemctl status porcupin

# View logs
sudo journalctl -u porcupin -f
```

---

## Docker

### Docker Compose (Recommended)

Create a `docker-compose.yml`:

```yaml
version: "3.8"
services:
    porcupin:
        image: ghcr.io/skullzarmy/porcupin:latest
        container_name: porcupin
        restart: unless-stopped
        volumes:
            - porcupin-data:/home/porcupin/.porcupin
        ports:
            - "4001:4001" # IPFS swarm (for sharing with network)
        environment:
            - TZ=America/New_York # Your timezone

volumes:
    porcupin-data:
```

```bash
# Start
docker-compose up -d

# Add a wallet
docker-compose exec porcupin porcupin --add-wallet tz1YourWalletAddress

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

### Docker Run

```bash
# Create data volume
docker volume create porcupin-data

# Run container
docker run -d \
  --name porcupin \
  --restart unless-stopped \
  -v porcupin-data:/home/porcupin/.porcupin \
  -p 4001:4001 \
  ghcr.io/skullzarmy/porcupin:latest

# Add a wallet
docker exec porcupin porcupin --add-wallet tz1YourWalletAddress

# View logs
docker logs -f porcupin
```

---

## Verify Installation

After installation, verify everything is working:

```bash
# Check version
porcupin --version

# Check status
porcupin --status

# List wallets
porcupin --list-wallets
```

---

## Next Steps

-   **[Quick Start Guide](quickstart.md)** - Add your first wallet
-   **[Configuration](configuration.md)** - Customize storage location and limits
-   **[Troubleshooting](troubleshooting.md)** - Common issues
