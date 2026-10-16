// Package app runs Boombrick's window, event loop and top-level navigation.
package app

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/ujjwal313/boombrick/assets"
	"github.com/ujjwal313/boombrick/internal/fixture"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/screens/home"
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
}

// App owns the window and dispatches input to the active tab.
type App struct {
	opts     Options
	window   *sdl.Window
	renderer *sdl.Renderer
	ui       *ui.Context
	mapper   input.Mapper
	repeat   *input.Repeater
	held     map[input.Button]bool
	tabs     []screens.Screen
	tab      int
	dirty    bool
	quit     bool
	drawn    bool
	wasAnim  bool
}

// New opens the window and builds the tabs.
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
	u.Status = fixture.Status()

	return &App{
		opts:     opts,
		window:   win,
		renderer: r,
		ui:       u,
		repeat:   input.NewRepeater(),
		held:     map[input.Button]bool{},
		dirty:    true,
		tabs: []screens.Screen{
			home.New(fixture.Home()),
			&screens.Placeholder{Tab: 1, Title: "Library"},
			&screens.Placeholder{Tab: 2, Title: "Spotify"},
			&screens.Placeholder{Tab: 3, Title: "YouTube"},
			&screens.Placeholder{Tab: 4, Title: "Downloads"},
		},
	}, nil
}

// Close releases SDL.
func (a *App) Close() {
	a.renderer.Destroy()
	a.window.Destroy()
	sdl.Quit()
}

// Run draws and handles input until quit. Frames are drawn only when
// something changed or a screen is animating; otherwise the loop sleeps until
// the next input, key repeat or clock minute.
func (a *App) Run() error {
	minute := -1
	for !a.quit {
		now := time.Now()
		if m := now.Minute(); m != minute {
			minute = m
			a.ui.Status.Time = fmt.Sprintf("%d:%02d", now.Hour(), now.Minute())
			a.dirty = true
		}
		screen := a.tabs[a.tab]
		anim := screen.Animating(now)
		// Draw once more when an animation stops so its settled frame shows.
		if a.dirty || anim || a.wasAnim {
			a.ui.Now = now
			screen.Draw(a.ui)
			a.ui.Canvas.Present()
			a.dirty = false
			if !a.drawn {
				a.drawn = true
				a.opts.Log.Info("first frame", "since_start", time.Since(a.opts.Started).Round(time.Millisecond))
			}
		}
		a.wasAnim = anim
		a.wait(now)
	}
	return nil
}

func (a *App) wait(now time.Time) {
	timeout := time.Until(now.Truncate(time.Minute).Add(time.Minute))
	if a.tabs[a.tab].Animating(now) {
		timeout = 0
	}
	if next, ok := a.repeat.Next(); ok {
		timeout = min(timeout, time.Until(next))
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
	if e.Kind == input.Press {
		switch e.Button {
		case input.L1:
			if a.tab > 0 {
				a.tab--
				a.dirty = true
			}
			return
		case input.R1:
			if a.tab < len(a.tabs)-1 {
				a.tab++
				a.dirty = true
			}
			return
		}
	}
	if a.tabs[a.tab].Handle(e, now) {
		a.dirty = true
	}
}
