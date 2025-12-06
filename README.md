# Porcupin 🦔

**A set-and-forget Tezos NFT preservation app that pins your NFT assets to IPFS.**

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.23+-00ADD8.svg)](https://go.dev/)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux%20%7C%20Docker-lightgrey.svg)](#user-installation)
[![Tezos](https://img.shields.io/badge/Tezos-NFT%20Preservation-2C7DF7.svg)](https://tezos.com)

Porcupin automatically monitors your Tezos wallets and backs up all NFT content (images, metadata, videos) to IPFS. Once configured, it runs in the background keeping your digital art collection safe—forever.

🌐 **Website:** [porcupin.xyz](https://porcupin.xyz)

---

## 🗳️ Support This Project

**Porcupin is seeking funding through the Tezos Ecosystem DAO!**

Help us build and maintain this open-source tool for the Tezos community. Vote on our proposal until **December 10th, 2025**:

👉 **[Vote on Tezos Homebase](https://tezos-homebase.io/explorer/lite/dao/64ef1c7d514de7b078cb8ed2/community/proposal/69305001ec807965f1487f61)**

_Connect any Tezos wallet to vote. Your Tez is your vote!_

---

## 📖 Documentation

| I want to...                    | Go here                                     |
| ------------------------------- | ------------------------------------------- |
| **Install and use Porcupin**    | **[User Guide](docs/user-guide/README.md)** |
| **Develop/contribute**          | [Contributing Guide](CONTRIBUTING.md)       |
| **Understand the architecture** | [Architecture Docs](docs/architecture.md)   |

---

## ✨ Features

-   🦔 **Set and Forget** — Add wallets once, Porcupin handles the rest
-   📌 **IPFS Pinning** — Embedded Kubo node, no external services needed
-   🔄 **Real-time Sync** — Watches for new NFTs via TZKT
-   💻 **Cross-Platform** — macOS, Windows, Linux, Raspberry Pi, Docker
-   📊 **Dashboard** — Track sync status, storage usage, failed assets
-   🔒 **Self-Sovereign** — Your data stays on your machine

---

## 🚀 User Installation

**👉 See the [User Guide](docs/user-guide/README.md) for complete installation instructions.**

### Quick Links

-   [Which binary do I need?](docs/user-guide/installation.md#quick-reference-which-binary-do-i-need)
-   [Desktop App](docs/user-guide/installation.md#desktop-app-gui) (macOS, Windows, Linux)
-   [Headless Server](docs/user-guide/installation.md#headless-server-no-gui) (Ubuntu, Raspberry Pi)
-   [Docker](docs/user-guide/installation.md#docker)
-   [Configuration](docs/user-guide/configuration.md)
-   [Troubleshooting](docs/user-guide/troubleshooting.md)

### Supported Platforms

| Platform              | Desktop (GUI) | Headless (CLI) | Docker |
| --------------------- | :-----------: | :------------: | :----: |
| macOS (Intel)         |      ✅       |       ✅       |   ✅   |
| macOS (Apple Silicon) |      ✅       |       ✅       |   ✅   |
| Windows x64           |      ✅       |       —        |   —    |
| Windows ARM64         |      ✅       |       —        |   —    |
| Linux x64             |      ✅       |       ✅       |   ✅   |
| Linux ARM64 (Pi)      |       —       |       ✅       |   ✅   |

---

## 🛠️ Developer Setup

For developers who want to build from source or contribute.

### Prerequisites

| Tool    | Version | Installation                                               |
| ------- | ------- | ---------------------------------------------------------- |
| Go      | 1.23+   | [go.dev/dl](https://go.dev/dl/)                            |
| Node.js | 18+     | [nodejs.org](https://nodejs.org/)                          |
| Wails   | v2      | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |

**Platform-specific:**

-   **macOS:** `xcode-select --install`
-   **Linux:** `sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.0-dev`
-   **Windows:** Visual Studio Build Tools with C++ workload

### Quick Start

```bash
git clone https://github.com/skullzarmy/porcupin-ipfs-backup-node.git
cd porcupin-ipfs-backup-node
npm install
npm run dev
```

### Build Commands

```bash
npm run build              # Build all (desktop + headless)
npm run build:desktop      # macOS + Windows desktop
npm run build:macos        # macOS only
npm run build:windows      # Windows only
npm run build:headless     # All headless binaries
npm run build:headless:linux  # Linux x64
npm run build:headless:arm    # Raspberry Pi ARM64
npm run build:docker       # Docker image
npm run clean              # Clean build artifacts
```

### Build Outputs

```text
porcupin/build/bin/
├── Porcupin.app/                  # macOS desktop
├── Porcupin.exe                   # Windows desktop
├── porcupin-server-linux-amd64    # Linux x64 headless
└── porcupin-server-linux-arm64    # Raspberry Pi headless
```

### Project Structure

```text
porcupin-ipfs-backup-node/
├── porcupin/                  # Main Wails application
│   ├── app.go                 # Wails bindings (Go ↔ JS)
│   ├── main.go                # Desktop entry point
│   ├── backend/               # Go backend
│   │   ├── config/            # Configuration
│   │   ├── core/              # BackupService, BackupManager
│   │   ├── db/                # SQLite (GORM)
│   │   ├── indexer/           # TZKT API
│   │   ├── ipfs/              # Embedded Kubo node
│   │   └── storage/           # Storage management
│   ├── cmd/headless/          # Headless server entry
│   └── frontend/              # React + Vite + TypeScript
├── docs/
│   ├── user-guide/            # 📖 User documentation
│   ├── architecture.md        # Technical architecture
│   └── requirements.md        # Product requirements
├── Dockerfile
└── docker-compose.yml
```

### Tests

```bash
cd porcupin
go test ./...
go test -cover ./...
```

---

## Architecture

See [docs/architecture.md](docs/architecture.md) for detailed technical documentation.

```text
┌─────────────────────────────────────────────────────────────┐
│                         Porcupin                            │
├─────────────────────────────────────────────────────────────┤
│  Frontend (React)          │  Backend (Go)                  │
│  ├── Dashboard             │  ├── BackupService             │
│  ├── Wallets               │  │   └── BackupManager         │
│  ├── Assets                │  ├── Indexer (TZKT API)        │
│  └── Settings              │  ├── IPFS Node (Kubo)          │
│                            │  └── Database (SQLite)         │
├─────────────────────────────────────────────────────────────┤
│              Wails v2 (Desktop) / CLI (Server)              │
└─────────────────────────────────────────────────────────────┘
```

---

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes and run tests
4. Submit a pull request

---

## 🤝 Contributing

We welcome contributions from developers of all skill levels! Please read our [Contributing Guide](CONTRIBUTING.md) before getting started.

**Quick links:**

-   [Code of Conduct](CONTRIBUTING.md#code-of-conduct)
-   [How to Contribute](CONTRIBUTING.md#how-can-i-contribute)
-   [Development Setup](CONTRIBUTING.md#development-setup)
-   [Style Guidelines](CONTRIBUTING.md#style-guidelines)

### Good First Issues

Look for issues labeled [`good first issue`](https://github.com/skullzarmy/porcupin-ipfs-backup-node/labels/good%20first%20issue) to get started!

---

## 📄 License

This project is licensed under the **MIT License** — see the [LICENSE](LICENSE) file for details.

You are free to use, modify, and distribute this software. We just ask that you give credit where it's due! 💜

---

## 🙏 Credits

### Built and Maintained by

<p align="center">
  <a href="https://fafolab.xyz">
    <img src="docs/assets/fafolab-logo.png" alt="FAFOlab" width="200"/>
  </a>
</p>

<p align="center">
  <strong>FAFO<s>lab</s></strong><br/>
  <a href="https://fafolab.xyz">fafolab.xyz</a> · <a href="mailto:info@fafolab.xyz">info@fafolab.xyz</a>
</p>

### Special Thanks

<p align="center">
  <strong>🏛️ Tezos Ecosystem DAO</strong><br/>
  Community-voted, community-funded infrastructure for the Tezos ecosystem.<br/>
  <em>This project is supported by the Tezos community.</em>
</p>

---

<p align="center">
  Made with 🦔💜 for the Tezos NFT community
</p>
