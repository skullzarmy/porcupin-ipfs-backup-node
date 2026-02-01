package main

import (
	"fmt"
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

// RestartApp restarts the application
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

	cmd := exec.Command(executable, args...)
	
	// Detach process IO
	// For GUI apps, sharing Stdout/Stdin can cause issues or hang the parent/child
	// We explicitly leave them nil to ensure independence
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart: %w", err)
	}
	
	// Exit the current process
	wailsRuntime.Quit(a.ctx)
	
	// Fallback exit if Quit (which is async cleanup) takes too long
	// We give it a moment to cleanup Wails resources, then force exit
	time.AfterFunc(2*time.Second, func() { os.Exit(0) })
	
	return nil
}
