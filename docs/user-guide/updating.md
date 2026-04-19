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

**Check and Install Updates:**

```bash
porcupin --update
```

**Expected Output (No Update):**

```text
Checking for updates...
Porcupin is up to date (version 1.0.1)
```

**Expected Output (Update Available):**

```text
Checking for updates...
New version available: v0.4.0
Release notes:
- Fixed critical IPFS bug
- Improved sync speed
Installing update... Success!
Please restart the application.
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
    # crontab -e
    0 3 * * * /usr/local/bin/porcupin --update >> /var/log/porcupin-update.log 2>&1
    ```

**How it works seamlessly:**

1.  The cron job runs `porcupin --update`.
2.  If an update is found, the `porcupin` tool replaces the binary on disk.
3.  The tool exits (successfully).
4.  Systemd observes the exit and, thanks to `Restart=always`, immediately spawns the _new_ binary.
5.  Service downtime is minimized to the time it takes the process to restart (seconds).

## 3. Technical Details

- **Library**: `github.com/creativeprojects/go-selfupdate`
- **Architecture**:
    - `backend/updater`: Shared logic for checking GitHub releases and verifying assets.
    - `backend/version`: Single source of truth for current version string.
    - `cmd/headless`: CLI wrapper invoking the updater.
    - `app.go` + `frontend`: GUI wrapper invoking the updater.

## Trouble Updating from v1.0.0 or Earlier

v1.0.1 fixed critical bugs in the self-updater. If you are running **v1.0.0 or earlier**, the built-in updater may not work correctly. Follow the manual upgrade instructions for your platform below.

### macOS (Desktop App)

v1.0.0's updater replaced only the inner binary inside the `.app` bundle, which invalidates the macOS code signature. The app will **bounce in the dock and refuse to open**. Additionally, v1.0.0 does not clean up IPFS lock files on shutdown, which can prevent the new version from starting its IPFS node.

**To upgrade manually:**

1. **Force quit Porcupin** if it's bouncing or stuck (⌘Q or right-click dock icon → Force Quit).

2. **Remove stale IPFS lock files** left behind by v1.0.0:

    ```bash
    rm -f ~/.porcupin/ipfs/repo.lock ~/.porcupin/ipfs/datastore/LOCK
    ```

3. **Download v1.0.1** (or later) from the [Releases page](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases).
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

From v1.0.1 onward, the updater downloads and swaps the full `.app` bundle, so future updates will work correctly.

### Windows (Desktop App)

The binary replacement updater generally works on Windows, but v1.0.0 does not clean up IPFS lock files on shutdown. If the app fails to start its IPFS node after updating, follow these steps:

1. **Close Porcupin** completely (check the system tray).

2. **Remove stale lock files** — open PowerShell and run:

    ```powershell
    Remove-Item -Force "$env:USERPROFILE\.porcupin\ipfs\repo.lock" -ErrorAction SilentlyContinue
    Remove-Item -Force "$env:USERPROFILE\.porcupin\ipfs\datastore\LOCK" -ErrorAction SilentlyContinue
    ```

3. **Restart Porcupin.** If it still fails, download `porcupin-windows-amd64.zip` (or `arm64`) from the [Releases page](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases), extract it, and replace your existing `porcupin.exe`.

### Linux (Desktop & Headless)

Same as Windows — the binary replacement updater works, but stale lock files may remain from v1.0.0.

1. **Stop Porcupin:**

    ```bash
    # Desktop: close the app
    # Headless/systemd:
    sudo systemctl stop porcupin
    ```

2. **Remove stale lock files:**

    ```bash
    rm -f ~/.porcupin/ipfs/repo.lock ~/.porcupin/ipfs/datastore/LOCK
    ```

3. **Restart.** If the updater itself failed, download the correct binary from the [Releases page](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases):
   - Desktop: `porcupin-linux-amd64.tar.gz`
   - Headless: `porcupin-server-linux-amd64` or `porcupin-server-linux-arm64`

    ```bash
    # Example for headless (adjust filename for your arch):
    chmod +x porcupin-server-linux-amd64
    sudo mv porcupin-server-linux-amd64 /usr/local/bin/porcupin
    sudo systemctl start porcupin
    ```

### Docker

Docker users are not affected by the updater — simply pull the latest image:

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
