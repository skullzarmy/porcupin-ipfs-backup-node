# Developer Setup Guide

This guide is for developers who want to build Porcupin from source or contribute to the project.

**Looking to install and use Porcupin?** See the [User Guide](docs/user-guide/README.md) instead.

---

## Prerequisites

### Required Tools

| Tool    | Version | Purpose                |
| ------- | ------- | ---------------------- |
| Go      | 1.23+   | Backend compilation    |
| Node.js | 18+     | Frontend build tooling |
| Wails   | v2      | Desktop app framework  |

### Install Go

**macOS:**

```bash
brew install go
```

**Linux (Ubuntu/Debian):**

```bash
wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

**Windows:**
Download from [go.dev/dl](https://go.dev/dl/) and run the installer.

### Install Node.js

**macOS:**

```bash
brew install node
```

**Linux (Ubuntu/Debian):**

```bash
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs
```

**Windows:**
Download from [nodejs.org](https://nodejs.org/) and run the installer.

### Install Wails

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

**Important:** Ensure `$(go env GOPATH)/bin` is in your PATH:

```bash
# Add to ~/.zshrc or ~/.bashrc
export PATH="$PATH:$(go env GOPATH)/bin"
source ~/.zshrc  # or ~/.bashrc
```

Verify installation:

```bash
wails version
```

### Platform-Specific Dependencies

**macOS:**

```bash
xcode-select --install
```

**Linux (Ubuntu/Debian):**

```bash
sudo apt-get install build-essential libgtk-3-dev libwebkit2gtk-4.0-dev
```

**Linux (Fedora):**

```bash
sudo dnf install gtk3-devel webkit2gtk4.0-devel
```

**Windows:**
Install [Visual Studio Build Tools](https://visualstudio.microsoft.com/visual-cpp-build-tools/) with "Desktop development with C++" workload.

---

## Clone and Run

```bash
# Clone
git clone https://github.com/skullzarmy/porcupin-ipfs-backup-node.git
cd porcupin-ipfs-backup-node

# Install npm dependencies (for build scripts)
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
│   └── requirements.md        # Product requirements
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

### Modifying IPFS Behavior

-   Pin/Unpin logic: `porcupin/backend/ipfs/node.go`
-   Backup orchestration: `porcupin/backend/core/backup.go`
-   Service lifecycle: `porcupin/backend/core/service.go`

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

---

## Troubleshooting Development Issues

### "wails: command not found"

Add Go bin to PATH:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Wails build fails on macOS

Install Xcode CLI tools:

```bash
xcode-select --install
```

### Wails build fails on Linux

Install GTK/WebKit:

```bash
# Ubuntu/Debian
sudo apt-get install libgtk-3-dev libwebkit2gtk-4.0-dev

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

-   **Go:** Follow standard Go formatting (`gofmt`)
-   **TypeScript/React:** Prettier + ESLint (configured in frontend)
-   Use meaningful commit messages
-   Add tests for new functionality

---

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make changes
4. Run tests: `cd porcupin && go test ./...`
5. Commit with descriptive message
6. Push and open a Pull Request
