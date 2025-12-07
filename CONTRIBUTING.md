# Contributing to Porcupin

First off, thank you for considering contributing to Porcupin! 🦔

Porcupin is an open-source project built for the Tezos NFT community. We welcome contributions from developers of all skill levels and backgrounds.

---

## Table of Contents

-   [Code of Conduct](#code-of-conduct)
-   [How Can I Contribute?](#how-can-i-contribute)
    -   [Reporting Bugs](#reporting-bugs)
    -   [Suggesting Features](#suggesting-features)
    -   [Your First Code Contribution](#your-first-code-contribution)
    -   [Pull Requests](#pull-requests)
-   [Development Setup](#development-setup)
-   [Style Guidelines](#style-guidelines)
    -   [Git Commit Messages](#git-commit-messages)
    -   [Go Style Guide](#go-style-guide)
    -   [TypeScript/React Style Guide](#typescriptreact-style-guide)
-   [Project Structure](#project-structure)
-   [Testing](#testing)
-   [Documentation](#documentation)
-   [Community](#community)

---

## Code of Conduct

This project and everyone participating in it is governed by our commitment to creating a welcoming and inclusive environment. By participating, you are expected to:

-   **Be respectful** - Treat everyone with respect. Absolutely no harassment, discrimination, or personal attacks.
-   **Be constructive** - Focus on what is best for the community and project.
-   **Be collaborative** - Work together openly and transparently.
-   **Be patient** - Remember that maintainers are volunteers. We'll respond as quickly as we can.

Unacceptable behavior may be reported to [info@fafolab.xyz](mailto:info@fafolab.xyz).

---

## How Can I Contribute?

### Reporting Bugs

Before creating a bug report, please check [existing issues](https://github.com/skullzarmy/porcupin-ipfs-backup-node/issues) to avoid duplicates.

When filing a bug report, include:

-   **Clear title** - Summarize the issue in a few words
-   **Environment** - OS, version, architecture (e.g., "macOS 14.1 ARM64")
-   **Steps to reproduce** - Detailed steps to reproduce the issue
-   **Expected behavior** - What you expected to happen
-   **Actual behavior** - What actually happened
-   **Logs** - Relevant log output (check `~/.porcupin/logs/`)
-   **Screenshots** - If applicable

Use the [Bug Report template](https://github.com/skullzarmy/porcupin-ipfs-backup-node/issues/new?template=bug_report.md) when available.

### Suggesting Features

We love hearing ideas for new features! Before suggesting:

1. Check [existing issues](https://github.com/skullzarmy/porcupin-ipfs-backup-node/issues) and [discussions](https://github.com/skullzarmy/porcupin-ipfs-backup-node/discussions)
2. Consider if it aligns with the project's goals (NFT preservation on IPFS)

When suggesting a feature:

-   **Clear title** - Summarize the feature
-   **Problem** - What problem does this solve?
-   **Solution** - How do you envision it working?
-   **Alternatives** - Any alternatives you've considered?
-   **Context** - Any additional context or mockups

### Your First Code Contribution

Unsure where to start? Look for issues labeled:

-   [`good first issue`](https://github.com/skullzarmy/porcupin-ipfs-backup-node/labels/good%20first%20issue) - Simple issues for newcomers
-   [`help wanted`](https://github.com/skullzarmy/porcupin-ipfs-backup-node/labels/help%20wanted) - Issues where we'd appreciate help
-   [`documentation`](https://github.com/skullzarmy/porcupin-ipfs-backup-node/labels/documentation) - Documentation improvements

### Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Follow style guidelines** (see below)
3. **Write tests** for new functionality
4. **Update documentation** if needed
5. **Ensure all tests pass** before submitting
6. **Write a clear PR description** explaining your changes

#### PR Checklist

-   [ ] I have read the [Contributing Guidelines](CONTRIBUTING.md)
-   [ ] My code follows the project's style guidelines
-   [ ] I have added tests for my changes
-   [ ] All new and existing tests pass
-   [ ] I have updated documentation if needed
-   [ ] My commits follow the commit message guidelines

---

## Development Setup

### Prerequisites

| Tool    | Version | Installation                                               |
| ------- | ------- | ---------------------------------------------------------- |
| Go      | 1.23+   | [go.dev/dl](https://go.dev/dl/)                            |
| Node.js | 18+     | [nodejs.org](https://nodejs.org/)                          |
| Wails   | v2      | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |

**Platform-specific dependencies:**

-   **macOS:** `xcode-select --install`
-   **Linux:** `sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev`
-   **Windows:** Visual Studio Build Tools with C++ workload

### Getting Started

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/porcupin-ipfs-backup-node.git
cd porcupin-ipfs-backup-node

# Add upstream remote
git remote add upstream https://github.com/skullzarmy/porcupin-ipfs-backup-node.git

# Install dependencies
npm install

# Start development server (hot reload)
npm run dev
```

### Building

```bash
npm run build              # Build all
npm run build:desktop      # Desktop apps
npm run build:headless     # Headless server binaries
npm run build:docker       # Docker image
```

---

## Style Guidelines

### Git Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Types:**

-   `feat` - New feature
-   `fix` - Bug fix
-   `docs` - Documentation only
-   `style` - Formatting, no code change
-   `refactor` - Code change that neither fixes a bug nor adds a feature
-   `perf` - Performance improvement
-   `test` - Adding or fixing tests
-   `chore` - Maintenance tasks

**Examples:**

```
feat(ipfs): add retry logic for failed pins
fix(indexer): handle rate limiting from TZKT API
docs(readme): update installation instructions
refactor(backup): extract asset processing to separate function
```

### Go Style Guide

-   Follow [Effective Go](https://golang.org/doc/effective_go)
-   Use `gofmt` and `goimports`
-   Run `go vet` before committing
-   Add comments for exported functions/types
-   Keep functions focused and concise
-   Handle errors explicitly (no silent failures)

```go
// Good
func (s *BackupService) ProcessAsset(ctx context.Context, asset *Asset) error {
    if asset == nil {
        return fmt.Errorf("asset cannot be nil")
    }
    // ...
}

// Bad
func (s *BackupService) ProcessAsset(asset *Asset) {
    // ignoring nil check and errors
}
```

### Cross-Platform Development

Porcupin supports macOS, Linux, and Windows. When adding platform-specific code:

-   **Use build tags** for code that won't compile on all platforms (syscalls, different imports)
-   **Use `runtime.GOOS` switches** for simple runtime differences
-   **Never use Unix commands** (`du`, `rsync`, `mount`) without a Windows alternative
-   **Always use `filepath.Join()`** instead of hardcoded path separators

See the full [Cross-Platform Development Guide](docs/cross-platform.md) for patterns and examples.

### TypeScript/React Style Guide

-   Use TypeScript strict mode
-   Prefer functional components with hooks
-   Use meaningful variable and function names
-   Keep components small and focused
-   Use proper prop types

```tsx
// Good
interface WalletCardProps {
    wallet: Wallet;
    onRemove: (address: string) => void;
}

export function WalletCard({ wallet, onRemove }: WalletCardProps) {
    // ...
}

// Bad
export function WalletCard(props: any) {
    // ...
}
```

---

## Project Structure

```
porcupin-ipfs-backup-node/
├── porcupin/                  # Main Wails application
│   ├── app.go                 # Wails bindings (Go ↔ JS)
│   ├── main.go                # Desktop entry point
│   ├── backend/               # Go backend
│   │   ├── config/            # Configuration management
│   │   ├── core/              # BackupService, BackupManager
│   │   ├── db/                # SQLite database (GORM)
│   │   ├── indexer/           # TZKT API integration
│   │   ├── ipfs/              # Embedded Kubo IPFS node
│   │   └── storage/           # Storage management
│   ├── cmd/headless/          # Headless server entry point
│   └── frontend/              # React + Vite + TypeScript
│       ├── src/
│       │   ├── components/    # React components
│       │   ├── hooks/         # Custom React hooks
│       │   ├── lib/           # Utilities
│       │   └── wailsjs/       # Generated Wails bindings
├── docs/                      # Documentation
│   ├── user-guide/            # End-user documentation
│   ├── architecture.md        # Technical architecture
│   └── requirements.md        # Product requirements
├── Dockerfile
├── docker-compose.yml
└── CONTRIBUTING.md            # This file
```

### Key Components

| Component     | Location                           | Purpose                                 |
| ------------- | ---------------------------------- | --------------------------------------- |
| BackupService | `porcupin/backend/core/service.go` | Orchestrates wallet sync lifecycle      |
| BackupManager | `porcupin/backend/core/backup.go`  | Handles NFT processing and IPFS pinning |
| Indexer       | `porcupin/backend/indexer/tzkt.go` | Fetches NFTs via TZKT API               |
| IPFS Node     | `porcupin/backend/ipfs/node.go`    | Embedded Kubo node                      |
| Database      | `porcupin/backend/db/db.go`        | SQLite with GORM                        |

---

## Testing

### Running Tests

```bash
cd porcupin

# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test ./backend/core/...

# Verbose output
go test -v ./...
```

### Writing Tests

-   Write tests for new functionality
-   Test edge cases and error conditions
-   Use table-driven tests where appropriate
-   Mock external dependencies (TZKT API, IPFS)

```go
func TestBackupManager_ProcessAsset(t *testing.T) {
    tests := []struct {
        name    string
        asset   *Asset
        wantErr bool
    }{
        {"valid asset", &Asset{URI: "ipfs://Qm..."}, false},
        {"nil asset", nil, true},
        {"empty URI", &Asset{URI: ""}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := manager.ProcessAsset(context.Background(), tt.asset)
            if (err != nil) != tt.wantErr {
                t.Errorf("ProcessAsset() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

---

## Documentation

Good documentation is essential! Please help us improve it.

### Types of Documentation

| Type          | Location               | Purpose                         |
| ------------- | ---------------------- | ------------------------------- |
| User Guide    | `docs/user-guide/`     | End-user installation and usage |
| Architecture  | `docs/architecture.md` | Technical design decisions      |
| Code Comments | Source files           | Inline documentation            |
| README        | `README.md`            | Project overview                |

### Documentation Guidelines

-   Use clear, concise language
-   Include code examples where helpful
-   Keep documentation up to date with code changes
-   Use proper Markdown formatting

---

## Community

-   **GitHub Issues** - Bug reports and feature requests
-   **GitHub Discussions** - General questions and ideas
-   **Email** - [info@fafolab.xyz](mailto:info@fafolab.xyz)
-   **Website** - [porcupin.xyz](https://porcupin.xyz)

---

## Recognition

Contributors are recognized in several ways:

-   Listed in release notes
-   GitHub contributor graph
-   Special recognition for significant contributions

---

## License

By contributing to Porcupin, you agree that your contributions will be licensed under the [MIT License](LICENSE).

---

Thank you for contributing to Porcupin! 🦔💜

_Built with love by FAFO~~lab~~ for the Tezos community_
