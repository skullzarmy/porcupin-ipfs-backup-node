# Data Backup & Recovery

Your Porcupin node protects your NFTs, but who protects your node? This guide covers how to back up your Porcupin installation.

---

## What to Backup?

Porcupin consists of two main parts:

### 1. The Brain: `porcupin.db` (CRITICAL)
- **What is it?** A SQLite database file containing your wallet list, tracking status, and the index of every file you've pinned.
- **Size:** Small (Megabytes).
- **Location:** `~/.porcupin/porcupin.db` (default).
- **Importance:** 🔴 **Start here.** If you lose this, you lose your tracking history and configuration.

### 2. The Body: IPFS Repo (Optional but Recommended)
- **What is it?** The actual image, video, and metadata files you've pinned.
- **Size:** Large (Gigabytes to Terabytes).
- **Location:** `~/.porcupin/ipfs` (default).
- **Importance:** 🟡 **Secondary.** Since these files exist on the public IPFS network (and potentially other gateways), they *can* be re-downloaded if you still have the database. However, re-syncing terabytes of data takes time.

---

## Strategy 1: "Backing up the Brain" (Recommended Minimum)

Since `porcupin.db` is a SQLite database, you cannot simply copy it while Porcupin is running (you might get a corrupted file).

### Method A: Stop and Copy
The safest way is to stop the service, copy the file, and restart.

```bash
# 1. Stop Porcupin
sudo systemctl stop porcupin

# 2. Copy database to a backup location (e.g., NAS, Cloud, another specific folder)
cp ~/.porcupin/porcupin.db /mnt/backup/porcupin-$(date +%F).db

# 3. Restart Porcupin
sudo systemctl start porcupin
```

### Method B: Online Backup (Advanced)
If you cannot stop the node, use the SQLite CLI to create a safe backup while running.

```bash
sqlite3 ~/.porcupin/porcupin.db ".backup '/mnt/backup/porcupin-hot.db'"
```

---

## Strategy 2: "Backing up the Body"

Backing up the `ipfs` directory is standard file backup. However, due to the millions of small files in IPFS datastores, standard copy commands (`cp`) can be very slow.

**Recommendation:** Use `rsync` for incremental backups to an external drive.

```bash
rsync -avh --delete ~/.porcupin/ipfs/ /mnt/external-drive/porcupin-ipfs-backup/
```

---

## ⚠️ Special Note for Flash Storage Users
*(Raspberry Pi SD Cards, USB Thumb Drives)*

Flash storage is prone to **corruption** on power loss and has limited write life.

1.  **Never Pull the Plug:** Always shut down gracefully (`sudo shutdown -h now`).
2.  **Off-Device Backup:** Do not back up to a folder on the same SD card. If the card dies, you lose both the live node and the backup. backup to:
    -   Network Attached Storage (NAS)
    -   Cloud Storage (rclone, Google Drive)
    -   A secondary USB drive
