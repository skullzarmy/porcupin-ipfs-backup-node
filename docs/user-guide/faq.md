# Frequently Asked Questions

## General

### What is Porcupin?

Porcupin is a desktop/server application that automatically backs up your Tezos NFT artwork to IPFS. It monitors your wallets and pins all NFT content (images, videos, metadata) to a local IPFS node.

### Why do I need this?

NFT artwork is stored on IPFS, a distributed network. But if no one is "pinning" (hosting) the files, they disappear. Marketplaces pin content while it's popular, but older or less popular NFTs may lose their hosts.

Porcupin ensures YOUR collection stays available by running your own IPFS node that pins everything you own.

### Is this the same as downloading my NFTs?

Similar, but better:

-   **Downloads:** Save files to your computer, but they're not on IPFS
-   **Porcupin:** Pins to IPFS AND shares with the network, so others can access them too

### Does Porcupin upload my NFTs somewhere?

No. Porcupin doesn't upload anything. It downloads existing IPFS content and pins it to your local node. Your node then shares with the IPFS network (peer-to-peer), but nothing goes to a central server.

---

## Storage & Resources

### How much disk space do I need?

Depends on your collection:

-   **Small collection (50 NFTs):** 1-5 GB
-   **Medium collection (500 NFTs):** 10-50 GB
-   **Large collection (5000+ NFTs):** 100+ GB

High-resolution images and videos take more space.

### Can I limit how much space Porcupin uses?

Yes! In `~/.porcupin/config.yaml`:

```yaml
backup:
    max_storage_gb: 100 # Stop at 100GB
```

### Can I use an external drive?

Yes. Change the IPFS repo path in config:

```yaml
ipfs:
    repo_path: /Volumes/ExternalDrive/porcupin-ipfs
```

### How much bandwidth does it use?

-   **Initial sync:** High (downloading all your NFTs)
-   **Ongoing:** Low (only new NFTs + occasional sharing)

IPFS also shares content with others, but this is minimal.

### Does it work on Raspberry Pi?

Yes! Porcupin runs great on Raspberry Pi 4 and 5.

For the Raspberry Pi Zero 2 W, specific optimizations are required.

👉 **See [Raspberry Pi Installation](installation.md#raspberry-pi-arm64) and [Minimal Hardware Setup](installation.md#minimal-hardware-raspberry-pi-zero-2-w) in the Installation Guide.**

---

## Wallets & Syncing

### What wallets can I add?

Any Tezos wallet address (tz1, tz2, tz3, KT1). You don't need the private key - Porcupin only reads public blockchain data.

### Does it sync NFTs I created or just ones I own?

Both by default! You can configure per-wallet:

-   **Owned:** NFTs currently in your wallet
-   **Created:** NFTs you minted (even if sold)

### How often does it check for new NFTs?

Every few minutes. When you receive or mint an NFT, it's usually backed up within 5-10 minutes.

### What if an NFT is "Failed (Unavailable)"?

This means the content isn't available on IPFS anymore. Porcupin will:

1. Mark it as failed
2. Periodically retry
3. Pin it if it becomes available again

Unfortunately, if no one has the content, it's lost. This is why Porcupin exists - to prevent this!

### What does "Skipped" mean?

Some NFTs have assets stored at HTTP/HTTPS URLs instead of IPFS. Since these can't be pinned to IPFS, Porcupin marks them as "Skipped". They are excluded from failure counts and don't trigger retries.

### How do I know if my IPFS node is connected?

The Dashboard shows a health indicator dot next to the navigation:
-   **Green** = IPFS node is online and connected to peers (with peer count displayed)
-   **Red** = IPFS node is offline or has no peers

You can also check connectivity from **Settings → Check Connectivity**.

### Can I sync someone else's wallet?

Technically yes - you can add any wallet address. But please be respectful of others' collections.

### Where are the logs?

-   **Desktop**: Go to **Settings → Logs & Diagnostics** to view recent logs with level filtering, export logs, or export a full diagnostic report.
-   **Log files on disk**: `~/.porcupin/logs/porcupin-YYYY-MM-DD.log`
-   **Crash reports**: `~/.porcupin/logs/crash-*.txt`
-   **Headless/systemd**: `journalctl -u porcupin -f`
-   **Docker**: `docker logs porcupin`

---

## Technical

### What is IPFS?

InterPlanetary File System - a distributed network for storing files. Instead of a central server, files are hosted by many computers. Each file has a unique address (CID) based on its content.

### What does "pinning" mean?

By default, IPFS nodes only keep files temporarily. "Pinning" tells your node to keep a file permanently. Porcupin pins all your NFT content so it stays available.

### Do I need to open ports?

Not required, but recommended for best sharing:

-   **Port 4001 (TCP/UDP):** IPFS swarm connections (configurable via `ipfs.swarm_port` in config or `--ipfs-port` CLI flag)

Without port forwarding, Porcupin still works but may share content less efficiently.

If port 4001 is already in use by another IPFS node, you can change it:

```yaml
# In ~/.porcupin/config.yaml
ipfs:
    swarm_port: 4002
```

Or via CLI: `porcupin --ipfs-port 4002`

### Is my data private?

NFT content is already public on the blockchain. Porcupin doesn't add any private data. Your wallet addresses are not secret - they're on the public blockchain.

### Where are the log files?

Porcupin writes daily log files and crash reports:

-   **Log files:** `~/.porcupin/logs/porcupin-YYYY-MM-DD.log`
-   **Crash reports:** `~/.porcupin/logs/crash-*.txt`
-   **Desktop app:** View logs in **Settings > Logs & Diagnostics**, or export a full diagnostic report
-   **Headless:** Check stdout/stderr or `journalctl -u porcupin`

### Can I run multiple instances?

Yes! You can run Porcupin on multiple computers with the same wallets. This provides:

-   Redundancy (if one goes offline)
-   Better sharing with the network
-   Faster access from different locations

---

## Comparison

### vs. Downloading to my computer

| Feature             | Download | Porcupin |
| ------------------- | -------- | -------- |
| Files on disk       | ✅       | ✅       |
| Available on IPFS   | ❌       | ✅       |
| Shares with others  | ❌       | ✅       |
| Auto-syncs new NFTs | ❌       | ✅       |

### vs. Commercial pinning services (Pinata, Filebase)

| Feature               | Commercial | Porcupin |
| --------------------- | ---------- | -------- |
| Monthly cost          | $5-100+    | Free     |
| No setup              | ✅         | ❌       |
| You control data      | ❌         | ✅       |
| Works offline         | ❌         | ✅       |
| Auto-syncs Tezos NFTs | ❌         | ✅       |

### vs. Running your own IPFS node

Porcupin IS an IPFS node, but with:

-   Automatic Tezos NFT discovery
-   Wallet tracking
-   Nice UI for management
-   "Set and forget" operation

---

## Troubleshooting

### The app won't start

See [Troubleshooting Guide](troubleshooting.md#installation-issues).

### Assets are stuck in "Pending"

See [Troubleshooting Guide](troubleshooting.md#assets-stuck-in-pending).

### I'm running out of disk space

Set a storage limit or use external storage. See [Configuration](configuration.md#limit-storage-usage).

---

## Other Questions?

-   [GitHub Discussions](https://github.com/skullzarmy/porcupin-ipfs-backup-node/discussions)
-   [Open an Issue](https://github.com/skullzarmy/porcupin-ipfs-backup-node/issues)
