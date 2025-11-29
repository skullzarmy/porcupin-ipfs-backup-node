# Porcupin Tezos Backup Node

A modern, optimized native application for preserving Tezos NFT history by backing up NFT data to IPFS.

## Overview

Porcupin connects to the Tezos blockchain via TZKT API, identifies NFTs associated with specific wallets (both owned and created), and ensures their content (metadata, images, assets) is pinned to IPFS, preventing data loss due to cache eviction or host disappearance.

## Features

- 🔐 **Complete NFT Preservation**: Backs up both metadata JSON and all referenced assets
- 🌐 **Multi-Platform**: Desktop (macOS, Windows, Linux), Raspberry Pi, and Docker
- ⚡ **Optimized Performance**: Event-driven WebSocket updates, concurrent processing
- 🛡️ **Production-Hardened**: Resource limits, security safeguards, validation
- 📊 **Real-time Dashboard**: Track sync status, pinned size, and node health

## Requirements

- Go 1.23+
- Wails v2 (for desktop builds)
- Docker (for containerized deployment)

## Quick Start

### Development Setup

```bash
# Install dependencies
go mod download

# Run in development mode
wails dev
```

### Docker Deployment

```bash
# Build the container
docker build -t porcupin .

# Run headless
docker run -v ./data:/data -p 127.0.0.1:8080:8080 porcupin
```

## Architecture

- **Backend**: Go (Golang) with embedded Kubo IPFS node
- **Frontend**: React with TailwindCSS
- **Database**: SQLite for local state
- **Desktop Wrapper**: Wails v2

## Documentation

See the `/docs` directory for detailed documentation:
- [Requirements](docs/requirements.md)
- [Architecture](docs/architecture.md)
- [Implementation Plan](docs/implementation_plan.md)
- [Security Audit](docs/audit.md)

## License

MIT License - See LICENSE file for details

## Contributing

Contributions are welcome! Please read CONTRIBUTING.md for guidelines.
