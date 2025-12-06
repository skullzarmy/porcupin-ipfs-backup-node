# Porcupin Build Directory

This directory contains build outputs and platform-specific assets for Porcupin.

## Structure

```
build/
├── bin/                              # Build outputs
│   ├── porcupin.app/                 # macOS desktop app
│   ├── porcupin.exe                  # Windows desktop app
│   ├── porcupin-server               # macOS headless
│   ├── porcupin-server-linux-amd64   # Linux x64 headless
│   └── porcupin-server-linux-arm64   # Raspberry Pi headless
├── darwin/                           # macOS-specific files
└── windows/                          # Windows-specific files
```

## Build Commands

From the project root:

```bash
# All platforms
npm run build

# Desktop only
npm run build:desktop

# Headless only (Linux/RPi)
npm run build:headless
```

Or from the `porcupin/` directory:

```bash
# Desktop (current platform)
wails build

# Windows
wails build -platform windows/amd64

# Headless
go build -o build/bin/porcupin-server ./cmd/headless
```

## Platform Files

### macOS (`darwin/`)

-   `Info.plist` - Bundle configuration for production builds
-   `Info.dev.plist` - Bundle configuration for development

These are auto-generated but can be customized. Delete to regenerate defaults.

### Windows (`windows/`)

-   `icon.ico` - Application icon (auto-generated from `appicon.png` if missing)
-   `info.json` - Metadata shown in file properties
-   `wails.exe.manifest` - Windows application manifest
-   `installer/` - NSIS installer scripts

## App Icon

The app icon is defined by `appicon.png` in the build directory. When building:

-   macOS: Converted to `.icns` automatically
-   Windows: Converted to `icon.ico` (or uses existing if present)

To update the icon, replace `appicon.png` and delete `icon.ico` to regenerate.
