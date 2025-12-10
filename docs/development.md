# Developer Guide

This guide is for developers who want to build Porcupin from source or contribute to the project.

**Looking to install and use Porcupin?** See the [User Guide](user-guide/README.md) instead.

---

## Prerequisites

### Required Tools

| Tool    | Version | Installation                                               |
| ------- | ------- | ---------------------------------------------------------- |
| Go      | 1.23+   | [go.dev/dl](https://go.dev/dl/)                            |
| Node.js | 18+     | [nodejs.org](https://nodejs.org/)                          |
| Wails   | v2      | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |

### Platform-Specific Dependencies

**macOS:**

```bash
xcode-select --install
```

**Linux (Ubuntu/Debian):**

```bash
# Ubuntu 24.04+
sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev

# Ubuntu 22.04
sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.0-dev
```

**Linux (Fedora):**

```bash
sudo dnf install gtk3-devel webkit2gtk4.0-devel
```

**Windows:**

Install [Visual Studio Build Tools](https://visualstudio.microsoft.com/visual-cpp-build-tools/) with "Desktop development with C++" workload.

---

## Quick Start

```bash
# Clone
git clone https://github.com/skullzarmy/porcupin-ipfs-backup-node.git
cd porcupin-ipfs-backup-node

# Install npm dependencies
npm install

# Run in development mode with hot reload
npm run dev
```

The app will open with hot-reloading enabled. Changes to Go or React code will auto-refresh.

---

## Build Commands

All builds are run from the repository root using npm scripts.

### Development

```bash
npm run dev          # Start dev mode with hot reload
```

### Production Builds

```bash
# Build everything
npm run build

# Desktop only
npm run build:desktop      # macOS + Windows
npm run build:macos        # macOS only
npm run build:windows      # Windows only

# Headless/Server only
npm run build:headless         # All platforms
npm run build:headless:linux   # Linux x64
npm run build:headless:arm     # Linux ARM64 (Raspberry Pi)

# Docker
npm run build:docker

# Clean
npm run clean
```

### Manual Wails Build

If you prefer running Wails directly:

```bash
cd porcupin

# Current platform
wails build

# Cross-compile
wails build -platform darwin/amd64    # macOS Intel
wails build -platform darwin/arm64    # macOS Apple Silicon
wails build -platform windows/amd64   # Windows
wails build -platform linux/amd64     # Linux
```

### Manual Headless Build

```bash
cd porcupin

# Current platform
go build -o build/bin/porcupin-server ./cmd/headless

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o build/bin/porcupin-server-linux-amd64 ./cmd/headless
GOOS=linux GOARCH=arm64 go build -o build/bin/porcupin-server-linux-arm64 ./cmd/headless
```

---

## Build Outputs

After building, binaries are in `porcupin/build/bin/`:

```text
porcupin/build/bin/
├── Porcupin.app/                  # macOS desktop (universal)
├── Porcupin.exe                   # Windows desktop
├── porcupin-server-linux-amd64    # Linux x64 headless
└── porcupin-server-linux-arm64    # Raspberry Pi headless
```

---

## Project Structure

```text
porcupin-ipfs-backup-node/
├── package.json               # npm build scripts
├── Dockerfile                 # Docker build
├── docker-compose.yml         # Docker compose config
│
├── porcupin/                  # Main application
│   ├── app.go                 # Wails bindings (exposed to JS)
│   ├── main.go                # Desktop entry point
│   ├── wails.json             # Wails config
│   │
│   ├── backend/               # Go backend code
│   │   ├── config/            # YAML config loading
│   │   ├── core/              # BackupService, BackupManager
│   │   ├── db/                # SQLite models (GORM)
│   │   ├── indexer/           # TZKT API client
│   │   ├── ipfs/              # Embedded Kubo node
│   │   └── storage/           # Storage detection & migration
│   │
│   ├── cmd/
│   │   └── headless/          # Headless server entry point
│   │
│   ├── frontend/              # React frontend
│   │   ├── src/               # React components
│   │   └── wailsjs/           # Auto-generated Go bindings
│   │
│   └── build/
│       └── bin/               # Build outputs
│
├── docs/
│   ├── user-guide/            # User documentation
│   ├── architecture.md        # Technical docs
│   └── development.md         # This file
│
└── backend/                   # (Future) Standalone backend
```

---

## Development Workflow

### Adding a Go Method (Exposed to Frontend)

1. Add method to `App` struct in `porcupin/app.go`:

    ```go
    func (a *App) MyNewMethod(arg string) (Result, error) {
        // implementation
    }
    ```

2. Wails auto-generates JS bindings on `wails dev` or `wails build`

3. Import in React:

    ```typescript
    import { MyNewMethod } from "../wailsjs/go/main/App";

    const result = await MyNewMethod("arg");
    ```

### Adding a Database Model

1. Define struct in `porcupin/backend/db/db.go`:

    ```go
    type MyModel struct {
        ID        uint64    `gorm:"primaryKey"`
        Name      string    `json:"name"`
        CreatedAt time.Time `json:"created_at"`
    }
    ```

2. Add to `InitDB()` AutoMigrate:

    ```go
    return db.AutoMigrate(&Wallet{}, &NFT{}, &Asset{}, &Setting{}, &MyModel{})
    ```

3. Create helper methods on `Database` struct

### Key Files for Common Changes

| Change                 | File(s)                                       |
| ---------------------- | --------------------------------------------- |
| IPFS pinning behavior  | `porcupin/backend/ipfs/node.go`               |
| Backup orchestration   | `porcupin/backend/core/backup.go`             |
| Service lifecycle      | `porcupin/backend/core/service.go`            |
| TZKT API integration   | `porcupin/backend/indexer/tzkt.go`            |
| Wails bindings (Go↔JS) | `porcupin/app.go`                             |
| Database models        | `porcupin/backend/db/db.go`                   |
| REST API endpoints     | `porcupin/backend/api/handlers.go`            |
| API middleware         | `porcupin/backend/api/middleware.go`          |
| Platform-specific code | See [Cross-Platform Guide](cross-platform.md) |

---

## Testing

```bash
cd porcupin

# Run all tests
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test ./backend/core/...

# Verbose
go test -v ./...
```

### Testing the REST API

The API can be tested manually with curl:

```bash
# Start server with a known token
export PORCUPIN_API_TOKEN="test_token_12345"
porcupin --serve

# In another terminal:

# Health check (no auth required)
curl http://localhost:8085/api/v1/health

# Get stats (auth required)
curl -H "Authorization: Bearer test_token_12345" http://localhost:8085/api/v1/stats

# Add wallet
curl -X POST -H "Authorization: Bearer test_token_12345" \
  -H "Content-Type: application/json" \
  -d '{"address":"tz1abc123"}' \
  http://localhost:8085/api/v1/wallets
```

API tests are in `porcupin/backend/api/api_test.go`.

---

## Troubleshooting

### "wails: command not found"

Add Go bin to PATH:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Add to `~/.zshrc` or `~/.bashrc` to persist.

### Wails build fails on macOS

Install Xcode CLI tools:

```bash
xcode-select --install
```

### Wails build fails on Linux

Install GTK/WebKit:

```bash
# Ubuntu 24.04+
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev

# Ubuntu 22.04
sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev

# Fedora
sudo dnf install gtk3-devel webkit2gtk4.0-devel
```

### "IPFS node fails to start" during development

Check if another IPFS node is using port 4001:

```bash
lsof -i :4001
```

Or check for stale lock file:

```bash
rm ~/.porcupin/ipfs/repo.lock
```

### Frontend changes not showing

Kill the dev server and restart:

```bash
# Ctrl+C to stop
npm run dev
```

---

## Code Style

-   **Go:** Follow standard Go formatting (`gofmt`, `goimports`)
-   **TypeScript/React:** Prettier + ESLint (configured in frontend)
-   Use meaningful commit messages ([Conventional Commits](https://www.conventionalcommits.org/))
-   Add tests for new functionality

See [CONTRIBUTING.md](../CONTRIBUTING.md) for full style guidelines.

---

## Related Documentation

| Document                                  | Description                                                  |
| ----------------------------------------- | ------------------------------------------------------------ |
| [Cross-Platform Guide](cross-platform.md) | Build tags, platform-specific patterns, and OS compatibility |
| [Architecture](architecture.md)           | System design, data flow, and component overview             |
| [Audit](audit.md)                         | Security and reliability risk mitigations                    |
| [CONTRIBUTING.md](../CONTRIBUTING.md)     | Contribution guidelines and code style                       |
