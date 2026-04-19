package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"porcupin/backend/updater"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// CheckForUpdates checks for available updates
func (a *App) CheckForUpdates() (*updater.UpdateInfo, error) {
	return a.updater.CheckForUpdates(a.ctx)
}

// InstallUpdate downloads and installs the latest update
func (a *App) InstallUpdate() error {
	// Phase 1: Downloading
	wailsRuntime.EventsEmit(a.ctx, "update:progress", updater.UpdateProgress{
		Phase:   "downloading",
		Message: "Downloading update from GitHub...",
		Percent: 0,
	})
	
	// Phase 2: Installing/Replacing
	// Note: InstallLatest blocks until done
	
	err := a.updater.InstallLatest(a.ctx)
	if err != nil {
		wailsRuntime.EventsEmit(a.ctx, "update:progress", updater.UpdateProgress{
			Phase:   "error",
			Message: "Update failed",
			Error:   err.Error(),
		})
		return err
	}
	
	// Phase 3: Complete
	wailsRuntime.EventsEmit(a.ctx, "update:progress", updater.UpdateProgress{
		Phase:   "complete",
		Message: "Update complete! Restarting...",
		Percent: 100,
	})
	
	return nil
}

// RestartApp restarts the application after an update.
// Shuts down the IPFS node and backup service first to release lock files,
// then spawns the new binary and exits.
func (a *App) RestartApp() error {
	wailsRuntime.EventsEmit(a.ctx, "app:restarting", true)

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	var args []string
	for _, arg := range os.Args[1:] {
		if arg != "--update" {
			args = append(args, arg)
		}
	}

	// Capture repo path before we nil the node reference.
	var repoPath string
	if a.ipfsNode != nil {
		repoPath = a.ipfsNode.GetRepoPath()
	}

	// Gracefully shut down services so lock files are released.
	if a.backupService != nil {
		a.backupService.Stop()
	}
	if a.ipfsNode != nil {
		if stopErr := a.ipfsNode.Stop(); stopErr != nil {
			slog.Error("Error stopping IPFS node before restart", "error", stopErr)
		}
		a.ipfsNode = nil
	}

	// Force-remove both lock files. In a restart we are the only legitimate
	// instance — the new process must be able to acquire these locks even if
	// Stop() timed out or failed to clean them up.
	if repoPath != "" {
		for _, rel := range []string{"repo.lock", filepath.Join("datastore", "LOCK")} {
			lf := filepath.Join(repoPath, rel)
			if err := os.Remove(lf); err != nil && !os.IsNotExist(err) {
				slog.Warn("Failed to remove lock file before restart", "path", lf, "error", err)
			}
		}
	}

	cmd := exec.Command(executable, args...)
	if runtime.GOOS == "darwin" {
		// Spawn a shell that waits for this process to exit, then opens the
		// .app bundle via Launch Services. Running `open -a` while the current
		// instance is still alive causes macOS to ignore the request.
		if bundle := updater.FindAppBundle(executable); bundle != "" {
			pid := os.Getpid()
			script := fmt.Sprintf(
				"while kill -0 %d 2>/dev/null; do sleep 0.2; done; open -a %q",
				pid, bundle,
			)
			cmd = exec.Command("sh", "-c", script)
		}
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart: %w", err)
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		wailsRuntime.Quit(a.ctx)
		time.AfterFunc(2*time.Second, func() { os.Exit(0) })
	}()

	return nil
}
