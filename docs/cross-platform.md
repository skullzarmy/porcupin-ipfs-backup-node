# Cross-Platform Development Guide

This document outlines the patterns and procedures for maintaining cross-platform compatibility across macOS, Linux, and Windows.

---

## Build Tag Convention

Use Go build tags to separate platform-specific code:

```go
//go:build !windows

package mypackage
// Unix-specific code (macOS + Linux)
```

```go
//go:build windows

package mypackage
// Windows-specific code
```

### File Naming

| Suffix        | Platforms    | Build Tag             |
| ------------- | ------------ | --------------------- |
| `_unix.go`    | macOS, Linux | `//go:build !windows` |
| `_windows.go` | Windows      | `//go:build windows`  |
| `_darwin.go`  | macOS only   | `//go:build darwin`   |
| `_linux.go`   | Linux only   | `//go:build linux`    |

**Example structure:**

```
storage/
├── storage.go           # Shared code, no build tags
├── types.go             # Shared types and orchestration
├── types_unix.go        # Unix implementations
├── types_windows.go     # Windows implementations
└── storage_test.go      # Shared tests
```

---

## Platform-Specific Implementations

### Directory Size Calculation

**Unix** (`du -sk`):

```go
//go:build !windows

func getDirSize(path string) (int64, error) {
    cmd := exec.Command("du", "-sk", path)
    output, err := cmd.Output()
    // Parse output: "12345\t/path/to/dir"
}
```

**Windows** (robocopy or PowerShell fallback):

```go
//go:build windows

func getDirSize(path string) (int64, error) {
    // Try robocopy first (faster)
    cmd := exec.Command("robocopy", path, "NUL", "/L", "/S", "/NJH", "/BYTES")
    // Fallback to PowerShell if needed
    cmd = exec.Command("powershell", "-Command",
        "(Get-ChildItem -Path '"+path+"' -Recurse | Measure-Object -Property Length -Sum).Sum")
}
```

### File Synchronization / Migration

**Unix** (`rsync`):

```go
//go:build !windows

func rsyncMigrate(src, dst string, progress func(int)) error {
    cmd := exec.Command("rsync", "-av", "--progress", src+"/", dst+"/")
    // Parse progress from stdout
}
```

**Windows** (`robocopy`):

```go
//go:build windows

func rsyncMigrate(src, dst string, progress func(int)) error {
    cmd := exec.Command("robocopy", src, dst, "/E", "/R:3", "/W:1")
    // robocopy exit codes: 0-7 are success, 8+ are errors
}
```

### Opening File Explorer

Use `runtime.GOOS` switch for simple cases:

```go
func ShowInFinder(path string) error {
    switch runtime.GOOS {
    case "darwin":
        return exec.Command("open", "-R", path).Run()
    case "windows":
        return exec.Command("explorer", "/select,", path).Run()
    default: // Linux
        return exec.Command("xdg-open", filepath.Dir(path)).Run()
    }
}
```

### Filesystem Operations

**Unix syscalls** (only in `_unix.go` files):

```go
//go:build !windows

import "syscall"

func getDiskStats(path string) (total, free uint64, err error) {
    var stat syscall.Statfs_t
    if err := syscall.Statfs(path, &stat); err != nil {
        return 0, 0, err
    }
    return stat.Blocks * uint64(stat.Bsize), stat.Bavail * uint64(stat.Bsize), nil
}
```

**Windows** (use `golang.org/x/sys/windows` or alternative approach):

```go
//go:build windows

import "golang.org/x/sys/windows"

func getDiskStats(path string) (total, free uint64, err error) {
    // Use windows.GetDiskFreeSpaceEx
}
```

---

## Common Patterns

### When to Use Build Tags vs runtime.GOOS

| Scenario                 | Approach                            |
| ------------------------ | ----------------------------------- |
| Different imports needed | Build tags (separate files)         |
| Different syscalls       | Build tags (separate files)         |
| External command varies  | Build tags OR `runtime.GOOS` switch |
| Simple path differences  | `runtime.GOOS` switch               |
| Different libraries      | Build tags (separate files)         |

**Rule of thumb**: If the code won't compile on all platforms, use build tags.

### Path Handling

Always use `filepath` package for cross-platform paths:

```go
import "path/filepath"

// Good
configPath := filepath.Join(homeDir, ".porcupin", "config.yaml")

// Bad - hardcoded separator
configPath := homeDir + "/.porcupin/config.yaml"
```

### Temp Directories

```go
import "os"

// Cross-platform temp directory
tmpDir := os.TempDir()
```

### Home Directory

```go
import "os"

homeDir, err := os.UserHomeDir()
```

---

## Testing

### Platform-Specific Tests

Use build tags for tests that only make sense on certain platforms:

```go
//go:build !windows

package storage

func TestUnixSpecificBehavior(t *testing.T) {
    // This test only runs on macOS/Linux
}
```

### Skipping Tests at Runtime

For tests that can compile everywhere but should only run on certain platforms:

```go
func TestLinuxOnly(t *testing.T) {
    if runtime.GOOS != "linux" {
        t.Skip("Linux-only test")
    }
    // Test code
}
```

### CI Matrix

The GitHub Actions workflow tests on all three platforms:

```yaml
strategy:
    matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
```

---

## Checklist for New Features

Before merging code that uses system commands or OS-specific APIs:

-   [ ] Does it use `exec.Command`? → Verify command exists on all platforms or use build tags
-   [ ] Does it use `syscall`? → Must be in platform-specific file with build tags
-   [ ] Does it use hardcoded paths (`/tmp`, `C:\`)? → Use `os.TempDir()`, `os.UserHomeDir()`, `filepath.Join()`
-   [ ] Does it shell out to Unix tools (`du`, `rsync`, `mount`)? → Create Windows alternative
-   [ ] Have you run `go build` targeting all platforms?

### Quick Cross-Compile Check

```bash
# From project root
GOOS=linux GOARCH=amd64 go build ./...
GOOS=darwin GOARCH=amd64 go build ./...
GOOS=windows GOARCH=amd64 go build ./...
```

---

## Reference: Platform-Specific Files in Codebase

| File                                                                                          | Purpose                                      |
| --------------------------------------------------------------------------------------------- | -------------------------------------------- |
| [porcupin/backend/storage/types_unix.go](../porcupin/backend/storage/types_unix.go)           | `getDirSize()`, `rsyncMigrate()` for Unix    |
| [porcupin/backend/storage/types_windows.go](../porcupin/backend/storage/types_windows.go)     | `getDirSize()`, `rsyncMigrate()` for Windows |
| [porcupin/backend/storage/storage_unix.go](../porcupin/backend/storage/storage_unix.go)       | Unix storage detection                       |
| [porcupin/backend/storage/storage_windows.go](../porcupin/backend/storage/storage_windows.go) | Windows storage detection                    |
| [porcupin/backend/core/disk_usage_unix.go](../porcupin/backend/core/disk_usage_unix.go)       | `getDiskUsageBytes()` for Unix               |
| [porcupin/backend/core/disk_usage_windows.go](../porcupin/backend/core/disk_usage_windows.go) | `getDiskUsageBytes()` for Windows            |

---

## Common Pitfalls

### ❌ Using Unix commands without build tags

```go
// BAD - compiles on Windows but crashes at runtime
func getSize(path string) int64 {
    cmd := exec.Command("du", "-sk", path)  // du doesn't exist on Windows
    ...
}
```

### ✅ Correct approach

```go
// In size_unix.go with //go:build !windows
func getSize(path string) int64 {
    cmd := exec.Command("du", "-sk", path)
    ...
}

// In size_windows.go with //go:build windows
func getSize(path string) int64 {
    cmd := exec.Command("powershell", "-Command", "...")
    ...
}
```

### ❌ Assuming path separators

```go
// BAD
path := baseDir + "/" + filename
```

### ✅ Correct approach

```go
// GOOD
path := filepath.Join(baseDir, filename)
```
