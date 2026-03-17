package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"
)

// WriteCrashReport writes a crash report file to dataDir/logs/crash-TIMESTAMP.txt.
// It includes the panic value, a goroutine stack dump, runtime info, and recent log entries.
func WriteCrashReport(dataDir string, r interface{}, ring *RingHandler) {
	logsDir := filepath.Join(dataDir, "logs")
	os.MkdirAll(logsDir, 0755)

	filename := "crash-" + time.Now().Format("2006-01-02T15-04-05") + ".txt"
	path := filepath.Join(logsDir, filename)

	f, err := os.Create(path)
	if err != nil {
		// Can't write crash report — nothing useful we can do
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "Porcupin Crash Report\n")
	fmt.Fprintf(f, "=====================\n\n")
	fmt.Fprintf(f, "Time:       %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "Go version: %s\n", runtime.Version())
	fmt.Fprintf(f, "OS/Arch:    %s/%s\n\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(f, "Panic value: %v\n\n", r)
	fmt.Fprintf(f, "Goroutine stack:\n%s\n\n", debug.Stack())

	if ring != nil {
		fmt.Fprintf(f, "Last 100 log entries:\n")
		fmt.Fprintf(f, "---------------------\n")
		for _, e := range ring.Entries(100, "") {
			fmt.Fprintf(f, "%s [%s] %s\n",
				e.Time.Format("2006-01-02T15:04:05.000"),
				e.Level,
				e.Message,
			)
		}
	}
}
