// Package app runs Boombrick's window, event loop and top-level navigation.
package app

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/ujjwal313/boombrick/assets"
	"github.com/ujjwal313/boombrick/internal/fixture"
	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/platform"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/resume"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/screens/browse"
	"github.com/ujjwal313/boombrick/internal/screens/home"
	"github.com/ujjwal313/boombrick/internal/screens/nowplaying"
	"github.com/ujjwal313/boombrick/internal/screens/queue"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/text"
)

// Options configure the app.
type Options struct {
	Fullscreen bool
	Log        *slog.Logger
	Started    time.Time // process start, for the launch-time log line
	Player     *player.Player
	MusicDir   string // shown by the empty library
	// Load reads the library and Progress the albums in progress; the app calls
	// them on a goroutine. Rescan asks for a scan. Scanned receives a value
	// after a scan that changed the library (and after the first scan), Saved
	// one after the saved progress moved on to another track or stopped.
	Load     func() (*library.Index, error)
	Progress func() ([]resume.Progress, error)
	Rescan   func()
	Scanned  <-chan struct{}
	Saved    <-chan struct{}
	// Status reads the battery and network for the status bar; the app calls
	// it on a goroutine every 10 s from start-up.
	Status func() platform.Status
}

// tick is how often the loop wakes while music plays: the progress bar
// updates 4 times a second.
const tick = 250 * time.Millisecond

// App owns the window and dispatches input to the active tab or Now Playing.
type App struct {
	opts       Options
	window     *sdl.Window
	renderer   *sdl.Renderer
	ui         *ui.Context
	mapper     input.Mapper
	repeat     *input.Repeater
	held       map[input.Button]bool
	home       *home.Home
	nowPlaying *nowplaying.NowPlaying
	library    *browse.Library
	queue      *queue.Queue
	shell      *shell
	ix         *library.Index
	last       player.State
	dirty      bool
	quit       bool
	drawn      bool
	scanEnded  bool // a scan has ended, so an empty index is the library
	wasAnim    bool

	loading bool // a load is running
	// another load was asked for while one ran, and whether it needs the index
	again, againIndex bool
	results           chan loaded
	scans, saves      atomic.Bool // a scan has ended or progress was saved since poll last looked

	statuses chan platform.Status // the latest status reading, for poll
	stop     chan struct{}        // closed by Close to end the status reader
	reader   sync.WaitGroup
	status   bool // a status reading has arrived

	mu     sync.Mutex // guards closed against wake and Quit racing Close
	closed bool
}

// loaded is a load's result. A nil index means only progress was read.
type loaded struct {
	ix       *library.Index
	progress []resume.Progress
	final    bool // a scan had ended when the load began
	took     time.Duration
	err      error
}

// New opens the window and builds the screens.
func New(opts Options) (*App, error) {
	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "1")
	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_JOYSTICK); err != nil {
		return nil, fmt.Errorf("sdl init: %w", err)
	}
	flags := uint32(sdl.WINDOW_SHOWN)
	if opts.Fullscreen {
		flags |= sdl.WINDOW_FULLSCREEN
	}
	win, err := sdl.CreateWindow("Boombrick", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, theme.DeviceW, theme.DeviceH, flags)
	if err != nil {
		sdl.Quit()
		return nil, fmt.Errorf("create window: %w", err)
	}
	r, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		opts.Log.Warn("accelerated renderer unavailable, using software", "err", err)
		if r, err = sdl.CreateRenderer(win, -1, sdl.RENDERER_SOFTWARE); err != nil {
			win.Destroy()
			sdl.Quit()
			return nil, fmt.Errorf("create renderer: %w", err)
		}
	}
	if info, err := r.GetInfo(); err == nil {
		opts.Log.Info("renderer", "name", info.Name)
	}
	r.SetLogicalSize(theme.DeviceW, theme.DeviceH)
	sdl.ShowCursor(sdl.DISABLE)
	for i := 0; i < sdl.NumJoysticks(); i++ {
		sdl.JoystickOpen(i)
	}

	fonts, err := text.NewEngine(assets.Fonts())
	if err != nil {
		r.Destroy()
		win.Destroy()
		sdl.Quit()
		return nil, err
	}
	u := ui.NewContext(gfx.New(r), fonts, theme.Scale)
	u.Accent = theme.Derive(fixture.PlayingHue, fixture.PlayingChroma)
	// Until the first reading, which comes within a frame or so: no battery,
	// and the Wi-Fi icon rather than the Offline warning.
	u.Status = ui.Status{Online: true, Battery: -1}

	a := &App{
		opts:       opts,
		window:     win,
		renderer:   r,
		ui:         u,
		repeat:     input.NewRepeater(),
		held:       map[input.Button]bool{},
		home:       home.New(nil),
		nowPlaying: nowplaying.New(opts.Player),
		queue:      queue.New(opts.Player),
		results:    make(chan loaded, 1),
		statuses:   make(chan platform.Status, 1),
		stop:       make(chan struct{}),
		dirty:      true,
	}
	if opts.Status != nil {
		a.reader.Add(1)
		go func() {
			defer a.reader.Done()
			watchStatus(opts.Status, statusEvery, a.statuses, a.wake, a.stop)
		}()
	}
	a.home.SetEmpty(opts.MusicDir, func() { a.rescan(); a.Toast(screens.Scanning) })
	a.library = browse.New(a, opts.Player, opts.MusicDir, a.rescan)
	forward := func(from <-chan struct{}, flag *atomic.Bool) {
		if from == nil {
			return
		}
		go func() {
			for range from {
				flag.Store(true)
				a.wake()
			}
		}()
	}
	forward(opts.Scanned, &a.scans)
	forward(opts.Saved, &a.saves)
	a.shell = newShell(a.home,
		a.library,
		&screens.Placeholder{Tab: 2, Title: "Spotify"},
		&screens.Placeholder{Tab: 3, Title: "YouTube"},
		&screens.Placeholder{Tab: 4, Title: "Downloads"})
	a.shell.overlays = []overlay{
		{button: input.Start, screen: a.nowPlaying, can: func() bool { return a.last.Queued }},
		{button: input.Select, screen: a.queue},
	}
	return a, nil
}

// Push opens s on the current tab.
func (a *App) Push(s screens.Screen) { a.shell.push(s, time.Now()); a.dirty = true }

// Pop closes the current tab's top page.
func (a *App) Pop() { a.shell.pop(time.Now()); a.dirty = true }

// Toast shows msg above the hint bar.
func (a *App) Toast(msg string) { a.shell.showToast(msg, time.Now()); a.dirty = true }

// Close stops the status reader and releases SDL.
func (a *App) Close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	close(a.stop)
	a.reader.Wait()
	a.renderer.Destroy()
	a.window.Destroy()
	sdl.Quit()
}

// Quit ends Run as if the window had been closed. It is safe to call from any
// goroutine.
func (a *App) Quit() { a.push(&sdl.QuitEvent{Type: sdl.QUIT}) }

// wake makes the loop's wait return. SDL_PushEvent may be called from any
// thread, but not once SDL has shut down.
func (a *App) wake() { a.push(&sdl.UserEvent{Type: sdl.USEREVENT}) }

func (a *App) push(ev sdl.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closed {
		sdl.PushEvent(ev)
	}
}

// Run draws and handles input until quit. Frames are drawn only when
// something changed, a screen is animating, or music is playing under Now
// Playing's progress bar; otherwise the loop sleeps until the next input, key
// repeat or clock minute.
func (a *App) Run() error {
	minute := -1
	for !a.quit {
		now := time.Now()
		if m := now.Minute(); m != minute {
			minute = m
			a.ui.Status.Time = fmt.Sprintf("%d:%02d", now.Hour(), now.Minute())
			a.dirty = true
		}
		a.poll()
		anim := a.animating(now)
		// Draw once more when an animation stops so its settled frame shows.
		if a.dirty || anim || a.wasAnim {
			a.draw(now)
			a.dirty = false
			if !a.drawn {
				a.drawn = true
				a.opts.Log.Info("first frame", "since_start", time.Since(a.opts.Started).Round(time.Millisecond))
				// The database holds last session's library; show it now and
				// refresh when a scan ends. Loading waits for the first frame.
				a.load(true)
			}
		}
		a.wasAnim = anim
		a.wait(now)
	}
	return nil
}

// poll picks up loaded data, the end of a library scan, the device status and
// player changes that show on screen.
func (a *App) poll() {
	select {
	case s := <-a.statuses:
		if !a.status {
			a.status = true
			a.opts.Log.Info("status", "battery", s.Battery, "online", s.Online)
		}
		a.ui.Status.Battery, a.ui.Status.Online, a.ui.Status.Bluetooth = s.Battery, s.Online, s.Bluetooth
		a.dirty = true
	default:
	}
	st := a.opts.Player.State()
	l := a.last
	if a.drawn {
		if a.scans.Swap(false) {
			a.scanEnded = true
			a.load(true)
		} else if a.saves.Swap(false) {
			a.load(false) // Continue listening moves on
		}
		select {
		case r := <-a.results:
			a.loading = false
			switch {
			case r.err != nil:
				a.opts.Log.Error("load library", "err", r.err)
			case r.ix != nil && len(r.ix.Albums) == 0 && !r.final:
				// The first scan hasn't ended, so an empty table says nothing yet
				// but that the scan is reading the music.
				a.home.SetReading()
				a.library.SetReading()
			default:
				if r.ix != nil {
					a.ix = r.ix
					a.library.SetIndex(r.ix)
					a.opts.Log.Info("loaded library", "albums", len(r.ix.Albums), "artists", len(r.ix.Artists),
						"songs", len(r.ix.Songs), "in_progress", len(r.progress), "took", r.took.Round(time.Millisecond))
				}
				a.showHome(r.progress)
			}
			if a.again {
				a.again = false
				index := a.againIndex
				a.againIndex = false
				a.load(index)
			}
			a.dirty = true
		default:
		}
	}
	if st.Err != nil && l.Err == nil {
		a.opts.Log.Warn("playback paused by an output error", "err", st.Err)
	}
	if st.Queued != l.Queued || st.Track != l.Track || st.Playing != l.Playing || st.Shuffle != l.Shuffle || st.Repeat != l.Repeat {
		a.ui.Status.NowPlaying = ""
		if st.Queued {
			a.ui.Status.NowPlaying = st.Track.Title
		}
		a.dirty = true
	}
	if a.shell.open >= 0 && st.Elapsed/tick != l.Elapsed/tick {
		a.dirty = true
	}
	a.last = st
}

// load reads the progress, and the index too when index is set, on a
// goroutine, then wakes the loop to show them.
func (a *App) load(index bool) {
	if a.loading {
		a.again, a.againIndex = true, a.againIndex || index
		return
	}
	a.loading = true
	final := a.scanEnded
	go func() {
		start := time.Now()
		r := loaded{final: final}
		if index {
			r.ix, r.err = a.opts.Load()
		}
		if r.err == nil {
			r.progress, r.err = a.opts.Progress()
		}
		r.took = time.Since(start)
		a.results <- r
		a.wake()
	}()
}

// showHome fills Home's rows from the index and the albums in progress.
func (a *App) showHome(progress []resume.Progress) {
	if a.ix == nil {
		return
	}
	var rows []fixture.Row
	if row, ok := home.ContinueRow(progress, a.ix, a.resumeAlbum, a.queueAlbum); ok {
		rows = append(rows, row)
	}
	if row, ok := home.AlbumRow(a.ix.Recent, a.playAlbum, a.queueAlbum); ok {
		rows = append(rows, row)
	}
	a.home.SetRows(rows)
}

func (a *App) playAlbum(al *library.Album) { playAlbum(a.opts.Player, al, 0, 0) }

func (a *App) resumeAlbum(al *library.Album, track int, frame int64) {
	playAlbum(a.opts.Player, al, track, frame)
}

// albumPlayer is what Home's albums need from the player.
type albumPlayer interface {
	SetShuffle(on bool)
	PlayFrom(tracks []player.Track, start int, frame int64)
}

// playAlbum plays al from track at frame, in order: shuffle left on by an
// album page's Shuffle pill doesn't carry over to albums started from Home.
func playAlbum(p albumPlayer, al *library.Album, track int, frame int64) {
	p.SetShuffle(false)
	p.PlayFrom(screens.Tracks(al.Tracks), track, frame)
}

func (a *App) queueAlbum(al *library.Album) {
	a.opts.Player.Enqueue(screens.Tracks(al.Tracks))
	a.Toast(screens.AddedToQueue)
}

func (a *App) rescan() {
	if a.opts.Rescan != nil {
		a.opts.Rescan()
	}
}

func (a *App) animating(now time.Time) bool {
	return a.shell.animating(now)
}

func (a *App) draw(now time.Time) {
	a.ui.Now = now
	a.shell.draw(a.ui)
	a.ui.Canvas.Present()
}

func (a *App) wait(now time.Time) {
	timeout := time.Until(now.Truncate(time.Minute).Add(time.Minute))
	if a.animating(now) {
		timeout = 0
	}
	if a.last.Playing {
		timeout = min(timeout, tick)
	}
	if next, ok := a.repeat.Next(); ok {
		timeout = min(timeout, time.Until(next))
	}
	if w, ok := a.shell.wake(now); ok {
		timeout = min(timeout, time.Until(w))
	}
	var ev sdl.Event
	if timeout <= 0 {
		ev = sdl.PollEvent()
	} else {
		ev = sdl.WaitEventTimeout(int(timeout/time.Millisecond) + 1)
	}
	for ; ev != nil; ev = sdl.PollEvent() {
		a.event(ev)
	}
	for _, e := range a.repeat.Due(time.Now()) {
		a.dispatch(e)
	}
}

func (a *App) event(ev sdl.Event) {
	var evs []input.Event
	switch e := ev.(type) {
	case *sdl.QuitEvent:
		a.quit = true
	case *sdl.KeyboardEvent:
		evs = a.mapper.Key(int(e.Keysym.Scancode), e.Type == sdl.KEYDOWN, e.Repeat != 0)
	case *sdl.JoyButtonEvent:
		evs = a.mapper.JoyButton(e.Button, e.State == sdl.PRESSED)
	case *sdl.JoyHatEvent:
		evs = a.mapper.JoyHat(e.Value)
	case *sdl.JoyAxisEvent:
		evs = a.mapper.JoyAxis(e.Axis, e.Value)
	case *sdl.WindowEvent:
		if e.Event == sdl.WINDOWEVENT_EXPOSED {
			a.dirty = true
		}
	}
	for _, e := range evs {
		a.dispatch(e)
	}
}

func (a *App) dispatch(e input.Event) {
	now := time.Now()
	switch e.Kind {
	case input.Press:
		a.held[e.Button] = true
		a.repeat.Press(e.Button, now)
	case input.Release:
		delete(a.held, e.Button)
		a.repeat.Release(e.Button)
	}
	// Select + Start quits until the quick menu, with its Quit entry, exists.
	if a.held[input.Select] && a.held[input.Start] {
		a.quit = true
		return
	}
	if a.shell.handle(e, now) {
		a.dirty = true
	}
}
