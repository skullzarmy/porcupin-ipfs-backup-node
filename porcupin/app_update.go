package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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
// It shuts down IPFS and the backup service first to release lock files,
// then spawns the new binary and exits.
func (a *App) RestartApp() error {
	wailsRuntime.EventsEmit(a.ctx, "app:restarting", true)

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Clean up arguments: remove existing --update flag if present to prevent loops
	var args []string
	for _, arg := range os.Args[1:] {
		if arg != "--update" {
			args = append(args, arg)
		}
	}

	// Shut down IPFS node and backup service BEFORE spawning the new process,
	// otherwise the new instance will fail to acquire the IPFS repo lock.
	if a.backupService != nil {
		a.backupService.Stop()
	}
	if a.ipfsNode != nil {
		if stopErr := a.ipfsNode.Stop(); stopErr != nil {
			slog.Error("Error stopping IPFS node before restart", "error", stopErr)
		}
		a.ipfsNode = nil
	}

	cmd := exec.Command(executable, args...)

	// Detach process IO
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart: %w", err)
	}

	// Give the new process a moment to begin initialization, then exit
	go func() {
		time.Sleep(500 * time.Millisecond)
		wailsRuntime.Quit(a.ctx)

		// Fallback exit if Quit takes too long
		time.AfterFunc(2*time.Second, func() { os.Exit(0) })
	}()

	return nil
}
