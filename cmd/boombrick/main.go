// Command boombrick is the music player for the TrimUI Brick.
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/ujjwal313/boombrick/internal/app"
	"github.com/ujjwal313/boombrick/internal/logx"
	"github.com/ujjwal313/boombrick/internal/platform"
	"github.com/ujjwal313/boombrick/internal/settings"
	"github.com/ujjwal313/boombrick/internal/store"
	"github.com/ujjwal313/boombrick/internal/version"
)

func init() {
	// SDL must be driven from the main OS thread.
	runtime.LockOSThread()
}

func main() {
	started := time.Now()
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}
	if err := run(started); err != nil {
		fmt.Fprintln(os.Stderr, "boombrick:", err)
		os.Exit(1)
	}
}

func run(started time.Time) error {
	// Keep the heap well inside the Brick's 1 GB, next to NextUI.
	debug.SetMemoryLimit(96 << 20)

	paths := platform.DetectPaths()
	for _, dir := range []string{paths.Data, paths.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	logFile, err := logx.Open(filepath.Join(paths.Logs, "Boombrick.txt"), 1<<20, 1)
	if err != nil {
		return err
	}
	defer logFile.Close()
	// On the device stderr is the crash file; keep the log out of it.
	var logOut io.Writer = logFile
	if !paths.OnDevice {
		logOut = io.MultiWriter(os.Stderr, logFile)
	}
	log := slog.New(slog.NewTextHandler(logOut, nil))
	log.Info("starting", "version", version.Version, "data", paths.Data)

	db, err := store.Open(filepath.Join(paths.Data, "boombrick.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := settings.Load(db.DB); err != nil {
		return err
	}

	a, err := app.New(app.Options{Fullscreen: paths.OnDevice, Log: log, Started: started})
	if err != nil {
		return err
	}
	defer a.Close()
	return a.Run()
}
