# Porcupin User Documentation

Welcome to Porcupin! This guide will help you install, configure, and use Porcupin to preserve your Tezos NFT assets.

## Table of Contents

1. **[Installation Guide](installation.md)** - Download and install Porcupin
2. **[Quick Start](quickstart.md)** - Get up and running in 5 minutes
3. **[CLI Reference](cli-reference.md)** - Command-line options for headless server
4. **[Remote Server Guide](remote-server.md)** - Run on NAS/Pi, manage from desktop
5. **[Advanced: Internet Exposure](advanced-exposure.md)** - ⚠️ For experts only
6. **[Configuration](configuration.md)** - Customize storage, limits, and behavior
7. **[Troubleshooting](troubleshooting.md)** - Common issues and solutions
8. **[FAQ](faq.md)** - Frequently asked questions

## What is Porcupin?

Porcupin is a **set-and-forget** application that automatically backs up your Tezos NFT artwork to IPFS. Once you add your wallet addresses, Porcupin:

-   📡 Monitors your wallets for new NFTs
-   📥 Downloads all artwork, videos, and metadata
-   📌 Pins everything to a local IPFS node
-   🌐 Shares content with the global IPFS network

**Why do you need this?** NFT artwork is stored on IPFS, but if no one is hosting (pinning) the files, they disappear. Porcupin ensures YOUR collection stays available forever.

## Which Version Should I Use?

| I want to...                            | Use This                        | Platform    |
| --------------------------------------- | ------------------------------- | ----------- |
| Run on my Mac with a GUI                | **Porcupin Desktop**            | macOS       |
| Run on my Windows PC with a GUI         | **Porcupin Desktop**            | Windows     |
| Run headless on an Ubuntu/Debian server | **porcupin-server-linux-amd64** | Linux x64   |
| Run on a Raspberry Pi                   | **porcupin-server-linux-arm64** | Linux ARM64 |
| Run in Docker on any server             | **Docker image**                | Any         |

→ See the **[Installation Guide](installation.md)** for download links and setup instructions.

## Quick Links

-   📦 [Latest Releases](https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases)
-   🐛 [Report a Bug](https://github.com/skullzarmy/porcupin-ipfs-backup-node/issues)
-   💬 [Discussions](https://github.com/skullzarmy/porcupin-ipfs-backup-node/discussions)
