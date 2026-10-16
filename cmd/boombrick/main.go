// Command boombrick is the music player for the TrimUI Brick.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/ujjwal313/boombrick/internal/app"
	"github.com/ujjwal313/boombrick/internal/art"
	"github.com/ujjwal313/boombrick/internal/audio/alsa"
	"github.com/ujjwal313/boombrick/internal/audio/decode"
	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/logx"
	"github.com/ujjwal313/boombrick/internal/lyrics"
	"github.com/ujjwal313/boombrick/internal/netx"
	"github.com/ujjwal313/boombrick/internal/platform"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/resume"
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
	prefs, err := settings.Load(db.DB)
	if err != nil {
		return err
	}

	// Album art and lyrics are found off the UI thread; what isn't with the
	// music is looked up online while the device is. They close after the
	// player, so the music stops at once on quitting, and before the database.
	web := netx.NewClient()
	online := func() bool { return platform.ReadStatus("/sys").Online }
	covers := art.New(art.Options{
		DB:     db.DB,
		Dir:    filepath.Join(paths.Data, "cache", "art"),
		Lookup: art.NewLookup(web),
		Online: online,
		Log:    log,
	})
	defer covers.Close()
	words := lyrics.New(lyrics.Options{DB: db.DB, Fetch: lyrics.NewClient(web), Online: online, Log: log})
	defer words.Close()

	p := player.New(alsa.New(), decode.Open, log)
	defer p.Close()
	p.SetLevelling(prefs.Bool(settings.VolumeLevelling))

	// Reading the saved queue and opening its track mustn't hold up the first
	// frame.
	restored := make(chan struct{})
	go func() {
		defer close(restored)
		if snap, ok, err := resume.Load(db.DB); err != nil {
			log.Warn("load saved queue", "err", err)
		} else if ok {
			p.Restore(snap)
		}
	}()
	saver := resume.NewSaver(db.DB)
	save := func() {
		if s, ok := p.Snapshot(); ok {
			if err := saver.Save(s); err != nil {
				log.Warn("save queue", "err", err)
			}
		}
	}
	defer save() // runs before the player and the database close

	// Quitting cancels the scanner and the saver, and waits for them and the
	// restore before the database closes under them.
	ctx, cancel := context.WithCancel(context.Background())
	scanDone, saved := make(chan struct{}), make(chan struct{})
	defer func() { cancel(); <-scanDone; <-saved; <-restored }()

	// Scans run one at a time on their own goroutine: at start-up, then each
	// time the app asks. Asking while a scan runs queues one more.
	formats := library.Formats{Supported: decode.Supported, Duration: decode.Duration}
	// The app hears when a scan writes to the library, at most every 10 s while
	// it runs, and when each scan ends.
	scanReq, scanned := make(chan struct{}, 1), make(chan app.ScanUpdate, 1)
	// tell hands u to the app, merged with one it hasn't taken yet. The
	// scanner is the only sender, so there is room after the drain.
	tell := func(u app.ScanUpdate) {
		select {
		case old := <-scanned:
			u.Changed, u.Ended = u.Changed || old.Changed, u.Ended || old.Ended
		default:
		}
		scanned <- u
	}
	scanReq <- struct{}{}
	go func() {
		defer close(scanDone)
		defer close(scanned)
		for {
			select {
			case <-ctx.Done():
				return
			case <-scanReq:
			}
			if ctx.Err() != nil {
				return
			}
			start := time.Now()
			var told time.Time
			wroteAny := false
			wrote := func(library.Stats) {
				wroteAny = true
				if time.Since(told) >= 10*time.Second {
					told = time.Now()
					tell(app.ScanUpdate{Changed: true})
				}
			}
			st, err := library.ScanWithProgress(ctx, db.DB, paths.Music, formats, wrote)
			changed := false
			switch {
			case errors.Is(err, context.Canceled):
				return
			case err != nil:
				log.Error("scan", "root", paths.Music, "err", err)
				changed = wroteAny // batches already written are worth loading
			default:
				log.Info("scanned", "root", paths.Music, "tracks", st.Total, "added", st.Added,
					"updated", st.Updated, "removed", st.Removed, "took", time.Since(start).Round(time.Millisecond))
				changed = st.Added+st.Updated+st.Removed > 0
			}
			tell(app.ScanUpdate{Changed: changed, Ended: true})
		}
	}()
	rescan := func() {
		select {
		case scanReq <- struct{}{}:
		default:
		}
	}
	// Save the queue every 5 s while music plays, so a crash or a flat battery
	// loses at most that much.
	// The app reloads Continue listening when a save moves to another track or
	// playback stops.
	savedTrack := make(chan struct{}, 1)
	go func() {
		defer close(saved)
		defer close(savedTrack)
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		wasPlaying := false
		lastPath := ""
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				// One more save when playback stops, to keep a paused position.
				st := p.State()
				if st.Playing || wasPlaying {
					save()
					if st.Track.Path != lastPath || !st.Playing {
						lastPath = st.Track.Path
						select {
						case savedTrack <- struct{}{}:
						default:
						}
					}
				}
				wasPlaying = st.Playing
			}
		}
	}()

	a, err := app.New(app.Options{
		Fullscreen: paths.OnDevice,
		Log:        log,
		Started:    started,
		Player:     p,
		MusicDir:   paths.Music,
		Load:       func() (*library.Index, error) { return library.Load(db.DB) },
		Progress:   func() ([]resume.Progress, error) { return resume.Recent(db.DB, 8) },
		Rescan:     rescan,
		Scanned:    scanned,
		Saved:      savedTrack,
		Status:     func() platform.Status { return platform.ReadStatus("/sys") },
		Covers:     covers,
		Lyrics:     words,
	})
	if err != nil {
		return err
	}
	defer a.Close()

	// A signal from the launcher ends the run like a quit, so the queue is saved.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	// The first signal quits; if that hangs, or a second signal comes, the
	// process exits regardless.
	go func() {
		<-sig
		a.Quit()
		select {
		case <-sig:
			log.Warn("second signal, exiting")
		case <-time.After(3 * time.Second):
			log.Error("quit timed out")
		}
		os.Exit(1)
	}()
	return a.Run()
}
