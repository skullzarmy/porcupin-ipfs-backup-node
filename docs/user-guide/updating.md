# Porcupin Update Mechanism

Porcupin supports automatic updates for both the Desktop GUI and the Headless Server. Updates are fetched directly from the GitHub repository releases.

## 1. Desktop Application

The Desktop application includes a built-in update Checker and Installer.

### How to Update

1.  Navigate to the **Settings** page.
2.  Locate the **Software Update** section.
3.  Click **Check for Updates**.
4.  If an update is available (e.g., `v1.1.0`), a modal will appear with release notes.
5.  Click **Update Now**.
6.  The application will download the new binary, replace the current one, and restart automatically.

### Configuration

- The updater checks the [GitHub Repository](https://github.com/skullzarmy/porcupin-ipfs-backup-node) for tags matching semantic versioning.
- It verifies the checksum of the downloaded binary against `checksums.txt` provided in the release assets.

## 2. Headless Server (CLI)

The headless server (`porcupin`) supports updates via the `--update` flag.

### Command Line Usage

When running as a systemd service, the binary is typically installed to `/usr/local/bin/porcupin` (owned by root) with data stored in `/var/lib/porcupin`. You must use `sudo` and specify the data directory:

```bash
# Stop the service first
sudo systemctl stop porcupin

# Run the update
sudo porcupin --data /var/lib/porcupin --update

# Remove any stale lock files (only needed if upgrading from < v1.0.2)
sudo rm -f /var/lib/porcupin/ipfs/repo.lock /var/lib/porcupin/ipfs/datastore/LOCK

# Restart the service
sudo systemctl start porcupin

# Verify
porcupin --version
sudo systemctl status porcupin
```

> **Why `sudo`?** The updater needs write access to `/usr/local/bin/porcupin`.
> **Why `--data`?** Without it, the updater defaults to `~/.porcupin`, which is the wrong location for systemd service deployments.

If you installed the binary elsewhere (e.g., `~/bin/porcupin`) and store data in `~/.porcupin`, you can run without sudo:

```bash
porcupin --update
```

**Expected Output (No Update):**

```text
Checking for updates...
Porcupin is up to date (version 1.0.2)
```

**Expected Output (Update Available):**

```text
Checking for updates...
New version available: 1.1.0
Release notes:
- Fixed critical IPFS bug
- Improved sync speed
Installing update... Success!
Please restart the application.
```

### Manual Update via curl

If the built-in updater is broken (e.g., upgrading from v1.0.0 or v1.0.1), you can download the correct binary directly:

```bash
sudo systemctl stop porcupin

# Download the latest server binary (amd64)
curl -L https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest/download/porcupin-server-linux-amd64 \
  -o /tmp/porcupin

# For ARM64 (Raspberry Pi, etc.):
# curl -L https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest/download/porcupin-server-linux-arm64 \
#   -o /tmp/porcupin

sudo mv /tmp/porcupin /usr/local/bin/porcupin
sudo chmod +x /usr/local/bin/porcupin
sudo rm -f /var/lib/porcupin/ipfs/repo.lock /var/lib/porcupin/ipfs/datastore/LOCK
sudo systemctl start porcupin
```

### Automation (Seamless Updates with Systemd)

To achieve a seamless update experience where the service automatically restarts after updating, follow these steps:

1.  **Install the Systemd Unit**:
    Copy the provided unit file to your system configuration:

    ```bash
    sudo cp docs/systemd/porcupin.service /etc/systemd/system/
    sudo systemctl daemon-reload
    sudo systemctl enable --now porcupin
    ```

2.  **Configure the Auto-Update Timer**:
    Create a cron job or systemd timer that runs the update check periodically (e.g., daily at 3 AM).
    ```bash
    # crontab -e (run as root)
    0 3 * * * systemctl stop porcupin && /usr/local/bin/porcupin --data /var/lib/porcupin --update && systemctl start porcupin >> /var/log/porcupin-update.log 2>&1
    ```

**How it works seamlessly:**

1.  The cron job stops the service, runs `porcupin --update`, then starts it again.
2.  If an update is found, the `porcupin` tool downloads and replaces the binary on disk.
3.  The service starts the new binary.
4.  Service downtime is minimized to the time it takes to download and restart (seconds to minutes).

## 3. Technical Details

- **Library**: `github.com/creativeprojects/go-selfupdate`
- **Architecture**:
    - `backend/updater`: Shared logic for checking GitHub releases and verifying assets.
    - `backend/version`: Single source of truth for current version string.
    - `cmd/headless`: CLI wrapper invoking the updater.
    - `app.go` + `frontend`: GUI wrapper invoking the updater.

## Trouble Updating from v1.0.1 or Earlier

Versions v1.0.0 and v1.0.1 had bugs in the self-updater that could install the wrong binary or leave stale lock files. **v1.0.2 fixes all known updater issues.** If the built-in updater doesn't work for you, follow the manual upgrade instructions for your platform below.

### macOS (Desktop App)

v1.0.0's updater replaced only the inner binary inside the `.app` bundle, which invalidates the macOS code signature. The app will **bounce in the dock and refuse to open**. Additionally, v1.0.0 does not clean up IPFS lock files on shutdown, which can prevent the new version from starting its IPFS node.

**To upgrade manually:**

1. **Force quit Porcupin** if it's bouncing or stuck (⌘Q or right-click dock icon → Force Quit).

2. **Remove stale IPFS lock files** left behind by v1.0.0:

    ```bash
    rm -f ~/.porcupin/ipfs/repo.lock ~/.porcupin/ipfs/datastore/LOCK
    ```

3. **Download the latest release** from the [Releases page](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases).
   Download `porcupin-macos.zip`.

4. **Replace the app:**

    ```bash
    # Remove the broken app
    rm -rf /Applications/Porcupin.app

    # Unzip the download (adjust path if your download location differs)
    unzip ~/Downloads/porcupin-macos.zip -d /Applications/

    # Clear quarantine so macOS doesn't block it
    xattr -cr /Applications/Porcupin.app
    ```

5. **Open Porcupin.** If macOS shows a security warning, right-click the app → Open → click "Open" in the dialog.

From v1.0.2 onward, the updater downloads and swaps the full `.app` bundle, so future updates will work correctly.

### Windows (Desktop App)

v1.0.0 does not clean up IPFS lock files on shutdown. If the app fails to start its IPFS node after updating, follow these steps:

1. **Close Porcupin** completely (check the system tray).

2. **Remove stale lock files.** Open PowerShell and run:

    ```powershell
    Remove-Item -Force "$env:USERPROFILE\.porcupin\ipfs\repo.lock" -ErrorAction SilentlyContinue
    Remove-Item -Force "$env:USERPROFILE\.porcupin\ipfs\datastore\LOCK" -ErrorAction SilentlyContinue
    ```

3. **Restart Porcupin.** If it still fails, download `porcupin-windows-amd64.zip` (or `arm64`) from the [Releases page](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases), extract it, and replace your existing `porcupin.exe`.

### Linux (Headless Server via systemd)

v1.0.0 and v1.0.1's updater could install the **wrong binary** (the desktop app instead of the headless server) because the update library matched release assets by OS/arch pattern rather than by name. v1.0.2 fixes this by downloading the correct asset (`porcupin-server-linux-{arch}`) explicitly.

**To upgrade manually:**

1. **Stop the service:**

    ```bash
    sudo systemctl stop porcupin
    ```

2. **Download and install the correct server binary:**

    ```bash
    # For x86_64 (most servers):
    curl -L https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest/download/porcupin-server-linux-amd64 \
      -o /tmp/porcupin

    # For ARM64 (Raspberry Pi, etc.):
    # curl -L https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/latest/download/porcupin-server-linux-arm64 \
    #   -o /tmp/porcupin

    sudo mv /tmp/porcupin /usr/local/bin/porcupin
    sudo chmod +x /usr/local/bin/porcupin
    ```

3. **Remove stale lock files and restart:**

    ```bash
    sudo rm -f /var/lib/porcupin/ipfs/repo.lock /var/lib/porcupin/ipfs/datastore/LOCK
    sudo systemctl start porcupin
    ```

4. **Verify:**

    ```bash
    porcupin --version
    sudo systemctl status porcupin
    ```

**Future updates** via `sudo porcupin --data /var/lib/porcupin --update` will work correctly from v1.0.2 onward.

> **Note:** The `--update` flag requires `sudo` because the binary lives in `/usr/local/bin/` (owned by root). The `--data` flag is required to point to the correct data directory used by the systemd service.

### Linux (Desktop App)

Before v1.0.4, the desktop updater could install the **wrong binary** on Linux. The update library matched release assets by OS/arch suffix, which also matches the headless server asset (`porcupin-server-linux-{arch}`). When it picked that one, the GUI binary was replaced by the headless server and **the app would no longer open**: it would exit immediately or start a server with no window.

**v1.0.4 fixes this** by downloading the desktop archive (`porcupin-linux-{arch}.tar.gz`) explicitly by name and verifying its SHA256 checksum before swapping the binary into place.

**To recover manually:**

1. **Close the app.**

2. **Remove stale lock files:**

    ```bash
    rm -f ~/.porcupin/ipfs/repo.lock ~/.porcupin/ipfs/datastore/LOCK
    ```

3. **If the updater failed or replaced the app with the headless server**, download `porcupin-linux-amd64.tar.gz` (or `arm64`) from the [Releases page](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases), extract it, and replace the existing binary:

    ```bash
    tar -xzf porcupin-linux-amd64.tar.gz
    chmod +x porcupin
    # Move it over your existing install, e.g.:
    mv porcupin ~/.local/bin/porcupin
    ```

**Future updates** from v1.0.4 onward download the correct desktop archive automatically.

### Docker

Docker users are unaffected by the updater. Simply pull the latest image:

```bash
docker pull ghcr.io/skullzarmy/porcupin-ipfs-backup-node:latest
docker compose up -d
```

---

## General Troubleshooting

**"Failed to update binary"**

- Ensure the user running the application has write permissions to the executable file.
- On Linux/macOS, check file ownership: `ls -l $(which porcupin)`.

**"Updater not initialized"**

- This usually means the build does not have connection to GitHub or the repository is private/inaccessible without a token (currently configured for public repo).
