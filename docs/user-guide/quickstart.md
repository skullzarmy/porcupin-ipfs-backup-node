# Quick Start Guide

Get Porcupin running and backing up your NFTs in 5 minutes.

---

## Desktop App

### Step 1: Launch Porcupin

Open the app. You'll see the Dashboard showing:

-   **Pinned**: 0 (number of backed up assets)
-   **Pending**: 0 (assets waiting to be pinned)
-   **Failed**: 0 (assets that couldn't be retrieved)

### Step 2: Add Your Wallet

1. Click the **Wallets** tab
2. Click **Add Wallet**
3. Enter your Tezos wallet address (e.g., `tz1abc...`)
4. Optionally add an alias (e.g., "Main Wallet")
5. Click **Add**

### Step 3: Watch the Magic

Porcupin will:

1. Query the Tezos blockchain for all your NFTs
2. Download metadata and artwork
3. Pin everything to IPFS

Watch the Dashboard as numbers go up! The first sync may take a while if you have many NFTs.

### Step 4: Set and Forget

That's it! Porcupin will:

-   **Run in the background** when you close the window (check system tray)
-   **Watch for new NFTs** automatically
-   **Retry failed downloads** periodically
-   **Share content** with the IPFS network

---

## Headless Server

### Step 1: Add a Wallet

```bash
porcupin --add-wallet tz1YourWalletAddress
```

Add as many wallets as you want:

```bash
porcupin --add-wallet tz1MainWallet
porcupin --add-wallet tz1ArtWallet
porcupin --add-wallet tz2CollectionWallet
```

### Step 2: Start the Daemon

```bash
porcupin
```

Porcupin will:

1. Start the embedded IPFS node
2. Sync all configured wallets
3. Keep running and watching for updates

### Step 3: Check Status

In another terminal:

```bash
# View statistics
porcupin --stats

# Output:
# Wallets: 3
# NFTs: 1,234
# Pinned: 5,678 assets (45.2 GB)
# Pending: 12
# Failed: 3
```

### Step 4: Run as a Service

For production, run as a systemd service (see [Installation Guide](installation.md#running-as-a-service-systemd)).

---

## Docker

### Step 1: Start the Container

```bash
docker-compose up -d
```

### Step 2: Add Wallets

```bash
docker-compose exec porcupin porcupin --add-wallet tz1YourWallet
```

### Step 3: Check Status

```bash
# View stats
docker-compose exec porcupin porcupin --stats

# Follow logs
docker-compose logs -f
```

---

## What Happens Next?

### Automatic Sync

-   Porcupin checks for new NFTs every few minutes
-   When you receive/mint a new NFT, it's automatically backed up

### Failed Assets

-   Some NFTs have broken/missing IPFS content
-   These are marked as "Failed (Unavailable)"
-   Porcupin periodically retries them in case they come back online

### Storage Growth

-   Your `~/.porcupin/ipfs` folder will grow as you pin more content
-   Monitor disk usage in the Dashboard or with `porcupin --stats`
-   Set storage limits in [Configuration](configuration.md)

---

## Tips

### Pin Only What You Need

By default, Porcupin pins both **owned** and **created** NFTs. You can configure this per-wallet:

**Desktop:** Edit wallet settings in the Wallets tab

**Headless:**

```bash
# Add wallet that only syncs owned NFTs (not created)
porcupin --add-wallet tz1YourWallet --sync-owned --no-sync-created
```

### External Storage

For large collections, consider:

-   **macOS**: External SSD or NAS
-   **Linux**: Mount an external drive to `~/.porcupin/ipfs`
-   **Docker**: Use a bind mount to a larger disk

See [Configuration](configuration.md) for details.

### Multiple Machines

You can run Porcupin on multiple machines with the same wallets. Each will:

-   Independently pin all content
-   Help serve content to the IPFS network
-   Provide redundancy if one machine goes offline

---

## Next Steps

-   **[Configuration](configuration.md)** - Storage limits, external drives, advanced settings
-   **[Troubleshooting](troubleshooting.md)** - Common issues
-   **[FAQ](faq.md)** - Frequently asked questions
