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
-   The updater checks the [GitHub Repository](https://github.com/skullzarmy/porcupin-ipfs-backup-node) for tags matching semantic versioning.
-   It verifies the checksum of the downloaded binary against `checksums.txt` provided in the release assets.

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
Porcupin is up to date (version 0.3.4-rc5)
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
4.  Systemd observes the exit and, thanks to `Restart=always`, immediately spawns the *new* binary.
5.  Service downtime is minimized to the time it takes the process to restart (seconds).

## 3. Technical Details

-   **Library**: `github.com/creativeprojects/go-selfupdate`
-   **Architecture**:
    -   `backend/updater`: Shared logic for checking GitHub releases and verifying assets.
    -   `backend/version`: Single source of truth for current version string.
    -   `cmd/headless`: CLI wrapper invoking the updater.
    -   `app.go` + `frontend`: GUI wrapper invoking the updater.

## Troubleshooting

**"Failed to update binary"**
-   Ensure the user running the application has write permissions to the executable file.
-   On Linux/macOS, check file ownership: `ls -l $(which porcupin)`.

**"Updater not initialized"**
-   This usually means the build does not have connection to GitHub or the repository is private/inaccessible without a token (currently configured for public repo).
