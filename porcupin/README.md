# Porcupin Desktop App

This is the Wails v2 desktop application for Porcupin - a Tezos NFT preservation tool that pins NFT assets to IPFS.

## Development

### Prerequisites

-   Go 1.23+
-   Node.js 18+
-   Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### Running in Development Mode

```bash
wails dev
```

This starts a Vite dev server with hot reload for the React frontend. The Go backend recompiles on changes.

To access Go methods from browser devtools, connect to http://localhost:34115.

### Project Structure

```
porcupin/
├── app.go              # Main Wails app, bindings to frontend
├── main.go             # Entry point
├── disk_unix.go        # Unix-specific disk space check
├── disk_windows.go     # Windows-specific disk space check
├── backend/
│   ├── config/         # Configuration loading
│   ├── core/           # BackupService, BackupManager
│   ├── db/             # SQLite database (GORM)
│   ├── indexer/        # TZKT API integration
│   └── ipfs/           # Embedded Kubo IPFS node
├── cmd/
│   └── headless/       # Headless server for Linux/Docker
├── frontend/           # React + Vite + TypeScript
│   ├── src/
│   │   ├── App.tsx     # Main app component
│   │   └── components/ # UI components
│   └── wailsjs/        # Auto-generated Wails bindings
└── build/
    └── bin/            # Build output directory
```

## Building

### Desktop (Current Platform)

```bash
wails build
```

Output: `build/bin/porcupin.app` (macOS) or `build/bin/porcupin.exe` (Windows)

### Cross-Platform Desktop

```bash
# Windows from macOS/Linux
wails build -platform windows/amd64

# macOS from macOS only (can't cross-compile)
wails build -platform darwin/universal
```

### Headless Server (Linux/Docker/RPi)

```bash
# Current platform
go build -o build/bin/porcupin-server ./cmd/headless

# Linux x64
GOOS=linux GOARCH=amd64 go build -o build/bin/porcupin-server-linux-amd64 ./cmd/headless

# Raspberry Pi (ARM64)
GOOS=linux GOARCH=arm64 go build -o build/bin/porcupin-server-linux-arm64 ./cmd/headless
```

## Adding New Features

### Exposing Go Methods to Frontend

1. Add method to `App` struct in `app.go`:

    ```go
    func (a *App) MyNewMethod(param string) (string, error) {
        return "result", nil
    }
    ```

2. Run `wails dev` to regenerate bindings

3. Import in React:

    ```typescript
    import { MyNewMethod } from "../wailsjs/go/main/App";

    const result = await MyNewMethod("param");
    ```

### Adding Database Models

1. Define struct in `backend/db/db.go` with GORM tags
2. Add to `InitDB()` AutoMigrate call
3. Create helper methods on `Database` struct

## Configuration

The app uses `~/.porcupin/config.yaml`. See `backend/config/config.go` for defaults.

## Wails Configuration

Edit `wails.json` to modify:

-   App name, dimensions, resizable settings
-   Frontend build commands
-   Asset embedding

Reference: https://wails.io/docs/reference/project-config
