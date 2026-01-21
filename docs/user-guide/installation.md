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
| **Linux Server (ARM64)**  | ARM64        | `porcupin-server-linux-arm64`  | Headless, Pi 4/5/Zero 2 W  |
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

> **Security Note:** On first launch, macOS may show "porcupin cannot be opened because the developer cannot be verified." Right-click the app and select "Open" to bypass this, or go to System Settings → Privacy & Security and click "Open Anyway". **See [Troubleshooting](troubleshooting.md#macos-porcupin-cant-be-opened-because-it-is-from-an-unidentified-developer) for more details.**

### Windows

1. Download `porcupin-windows-amd64.zip` (or `arm64` for ARM devices) from [Releases](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest)
2. Extract and run `porcupin.exe`
3. Allow network access when Windows Firewall prompts
4. Add your wallet addresses and you're done!

**System Requirements:**

-   Windows 10/11 (64-bit)
-   4GB RAM minimum
-   10GB+ free disk space

> **Firewall Note:** On first launch, Windows Firewall will ask "Allow porcupin to communicate on networks?" Click **Allow** for both private and public networks. This is required for IPFS to connect to peers and for remote server connections.

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

> **Firewall Note:** If using `ufw`, you may need to allow IPFS ports:
>
> ```bash
> sudo ufw allow 4001/tcp  # IPFS swarm
> sudo ufw allow 4001/udp  # IPFS swarm (QUIC)
> ```
>
> If running the headless server with remote access enabled, also allow the API port:
>
> ```bash
> sudo ufw allow 8085/tcp  # Porcupin API (only if using remote access)
> ```

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
### Minimal Hardware (Raspberry Pi Zero 2 W)
    
The Raspberry Pi Zero 2 W uses a quad-core processor but is severely limited by its **512MB RAM**. To run Porcupin (which includes a full IPFS node) stably, you must perform the following optimizations.

> [!CAUTION]
> **Hardware Limitations:** While we have made every effort to support the Zero 2 W and have proven functionality in testing, we are working at the extreme edge of the device's capabilities. We experienced multiple issues and instability during development. We **strongly recommend** opting for a more powerful device (like a Raspberry Pi 4 or 5) for a stable experience.

**1. Enable Swap (CRITICAL)**

The default 100MB swap is insufficient. You need at least 2GB of swap or the process will be killed (OOM).

**Where should I put the swap file?**
*    **USB SSD (Recommended):** fast and saves your SD card from burning out.
*    **USB Flash Drive:** Saves your SD card, but might be slow.
*    **SD Card (Fallback):** Simplest, but heavy swapping will wear it out faster.

**Option A: Swap on USB (Recommended)**
Assuming your drive is mounted at `/mnt/usb` and formatted as ext4:
```bash
sudo fallocate -l 2G /mnt/usb/swapfile
sudo chmod 600 /mnt/usb/swapfile
sudo mkswap /mnt/usb/swapfile
sudo swapon /mnt/usb/swapfile
echo '/mnt/usb/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

**Option B: Swap on SD Card**
```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

**2. Optimize Configuration**

The config file is generated automatically the first time you run Porcupin. You can also create it manually.

Edit `~/.porcupin/config.yaml` (or `/var/lib/porcupin/config.yaml` if running as a service) and paste the following low-memory tuning:

```yaml
backup:
  max_concurrency: 1           # Default is 3. Limit to 1 worker on Zero 2 W.
  max_metadata_size_mb: 1      # Avoid loading huge JSONs into RAM
  min_free_disk_space_gb: 1    # Lower requirement for small SD cards
  max_storage_gb: 0            # 0 = unlimited

ipfs:
  rate_limit_mbps: 2           # Prevent network buffers from filling RAM
  repo_path: "~/.porcupin/ipfs"

server:
  bind_address: "127.0.0.1:8080"
  enable_auth: false

tzkt:
  base_url: "https://api.tzkt.io"

api:
  enabled: false # Enable if managing remotely
  port: 8085
```

**3. Runtime Tuning (GOGC)**

When running the service, force Go to garbage collect more aggressively by setting `GOGC`.

Manually:
```bash
GOGC=20 porcupin ...
```

Or in your systemd service file (`/etc/systemd/system/porcupin.service`):
```ini
[Service]
Environment="GOGC=20"
...
```

3.  **High Endurance Storage:** If using a USB Flash Drive or SD Card, ensure it is "High Endurance" or "Pro" grade.
    -   *Warning:* USB 2.0 speeds will limit sync performance.
    -   *Warning:* Random I/O on cheap flash drives may cause system stalls (unresponsiveness).

### External Storage Setup (Crucial)

If you are using an external USB drive (highly recommended for Raspberry Pi), **you must format it as ext4**. Windows filesystems like FAT32 or exFAT **do not support** the file permissions required by IPFS and will cause errors.

**1. Format the Drive (ext4):**

*Warning: This will erase all data on the drive!*

```bash
# Find your drive (e.g., /dev/sda)
lsblk

# Format partition 1 (e.g., /dev/sda1)
sudo mkfs.ext4 /dev/sda1
```

**2. Configure Auto-Mount (`/etc/fstab`):**

To ensure the drive mounts automatically on boot:

```bash
# Get the UUID of the new partition
sudo blkid /dev/sda1
# Output example: UUID="96106d19-..." BLOCK_SIZE="4096" TYPE="ext4"

# Create mount point
sudo mkdir -p /mnt/usb

# Edit fstab
sudo nano /etc/fstab

# Add this line (replace UUID with yours):
UUID=your-uuid-here /mnt/usb ext4 defaults,noatime 0 2
```

**3. Set Permissions:**

After mounting, ensure the `porcupin` user owns the directory:

```bash
sudo mount -a
sudo chown -R porcupin:porcupin /mnt/usb
```

### Headless Wi-Fi Setup

Since the Pi Zero 2 W has no Ethernet port, you must configure Wi-Fi before first boot:

1.  Flash your SD card with Raspberry Pi OS Lite.
2.  On your computer, open the `boot` partition of the SD card.
3.  Create a file named `wpa_supplicant.conf` with the following content:

```text
country=US
ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev
update_config=1

network={
    ssid="YOUR_WIFI_NAME"
    psk="YOUR_WIFI_PASSWORD"
}
```

4.  (Optional) Create an empty file named `ssh` (no extension) in the same `boot` partition to enable SSH.

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
sudo useradd -r -d /var/lib/porcupin -s /bin/false porcupin
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

### Interacting with the Service

When running as a systemd service, the data is stored in `/var/lib/porcupin`. You **must** specify this directory when running manual commands, and ideally run them as the `porcupin` user:

```bash
# Add a wallet
sudo -u porcupin porcupin --data /var/lib/porcupin --add-wallet tz1YourWallet

# Check status
sudo -u porcupin porcupin --data /var/lib/porcupin --stats

# List wallets
sudo -u porcupin porcupin --data /var/lib/porcupin --list-wallets
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
