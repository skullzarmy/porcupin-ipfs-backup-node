# Porcupin Development Setup

## Prerequisites

### Go Installation
```bash
# macOS (via Homebrew)
brew install go

# Linux
wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Windows
# Download and install from https://go.dev/dl/
```

### Wails Installation
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Node.js (for frontend)
```bash
# macOS
brew install node

# Linux
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs
```

## Project Initialization

Once you have Go installed, initialize the project:

```bash
# Initialize Go module
go mod init github.com/porcupin/porcupin

# Initialize Wails project (if not already initialized)
wails init -n porcupin -t react-ts

# Install Go dependencies
go get github.com/wailsapp/wails/v2
go get gorm.io/gorm
go get gorm.io/driver/sqlite
go get github.com/ipfs/kubo

# Install frontend dependencies
cd frontend
npm install
cd ..
```

## Running the Application

### Development Mode
```bash
wails dev
```

### Building for Production
```bash
# Build for current platform
wails build

# Build for specific platforms
wails build -platform darwin/universal  # macOS Universal
wails build -platform windows/amd64     # Windows 64-bit
wails build -platform linux/amd64       # Linux 64-bit
```

## Docker Setup

Build and run the headless version:
```bash
docker build -t porcupin .
docker run -v $(pwd)/data:/data -p 127.0.0.1:8080:8080 porcupin
```

## Project Structure

```
/
├── backend/
│   ├── cmd/               # Entry points
│   ├── internal/
│   │   ├── core/          # Domain logic
│   │   ├── db/            # Database access
│   │   ├── indexer/       # Tezos/TZKT logic
│   │   ├── ipfs/          # IPFS logic
│   │   ├── api/           # HTTP/Wails handlers
│   │   └── config/        # Configuration
│   └── pkg/               # Shared utilities
├── frontend/              # React app
│   ├── src/
│   └── public/
├── build/                 # Build artifacts
├── docker/                # Dockerfile & Compose
├── docs/                  # Documentation
├── go.mod
└── wails.json
```

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run integration tests
go test -tags=integration ./...
```

## Troubleshooting

### Go not found
Ensure Go is in your PATH:
```bash
export PATH=$PATH:/usr/local/go/bin
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.zshrc
```

### Wails build fails
Ensure all platform-specific dependencies are installed:
```bash
# macOS - requires Xcode Command Line Tools
xcode-select --install

# Linux - requires build essentials
sudo apt-get install build-essential libgtk-3-dev libwebkit2gtk-4.0-dev
```
