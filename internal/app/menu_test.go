package app

import (
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
)

// fakeMenu records what reaches it while open.
type fakeMenu struct {
	page
	open bool
	due  time.Time // when its sleep timer next changes; zero for none
}

func (m *fakeMenu) Open(time.Time)                  { m.open = true }
func (m *fakeMenu) IsOpen() bool                    { return m.open }
func (m *fakeMenu) Visible(time.Time) bool          { return m.open }
func (m *fakeMenu) Due(time.Time) (time.Time, bool) { return m.due, !m.due.IsZero() }

func (m *fakeMenu) Handle(e input.Event, now time.Time) bool {
	m.got = append(m.got, e.Button)
	if e.Button == input.B {
		m.open = false
	}
	return true
}

// menuClaimer keeps Menu for itself, as a screen with a dialog open does.
type menuClaimer struct{ page }

func (c *menuClaimer) Claims(b input.Button) bool { return b == input.Menu }

func TestMenuOpensTheQuickMenu(t *testing.T) {
	s, tabs, np := testShell()
	m := &fakeMenu{}
	s.menu = m
	if s.handle(press(input.Menu), t0) || m.open {
		t.Fatal("the quick menu opened before Menu was let go")
	}
	if !s.handle(release(input.Menu), t0) || !m.open {
		t.Fatal("letting Menu go didn't open the quick menu")
	}
	for _, b := range []input.Button{input.L1, input.Start, input.A, input.B} {
		s.handle(press(b), t0)
	}
	if !slices.Equal(m.got, []input.Button{input.L1, input.Start, input.A, input.B}) || len(tabs[0].got) > 0 || s.open >= 0 || s.tab != 0 {
		t.Fatalf("with the menu open: menu got %v, tab %v, overlay %d, tab %d", m.got, tabs[0].got, s.open, s.tab)
	}
	// Over an overlay too.
	s.handle(press(input.Start), t0)
	s.handle(press(input.Menu), t0)
	s.handle(release(input.Menu), t0)
	if !m.open || s.overlay() != np {
		t.Fatal("Menu over Now Playing didn't open the menu over it")
	}
}

// NextUI sets the brightness with Menu held and the volume buttons: that
// chord doesn't open the quick menu.
func TestAMenuChordDoesNotOpenTheMenu(t *testing.T) {
	s, _, _ := testShell()
	m := &fakeMenu{}
	s.menu = m
	for _, e := range []input.Event{press(input.Menu), press(input.VolUp), release(input.VolUp), release(input.Menu)} {
		s.handle(e, t0)
	}
	if m.open {
		t.Fatal("Menu with volume up opened the quick menu")
	}
	s.handle(press(input.Menu), t0)
	s.handle(release(input.Menu), t0)
	if !m.open {
		t.Fatal("Menu alone, after a chord, didn't open it")
	}
}

func TestAScreenCanKeepMenu(t *testing.T) {
	tab := &menuClaimer{page{name: "downloads"}}
	s := newShell(tab)
	m := &fakeMenu{}
	s.menu = m
	s.handle(press(input.Menu), t0)
	if m.open || !slices.Equal(tab.got, []input.Button{input.Menu}) {
		t.Fatal("Menu went past the screen keeping it")
	}
	s.handle(release(input.Menu), t0)
	if m.open {
		t.Fatal("letting go of Menu kept by the screen opened the quick menu")
	}
}

// An overlay may keep a button, as Settings keeps Start with its dialog open.
type startKeeper struct{ page }

func (k *startKeeper) Claims(b input.Button) bool { return b == input.Start }

func TestAnOverlayCanKeepAButton(t *testing.T) {
	s := newShell(&page{name: "home"})
	keeper := &startKeeper{page{name: "settings"}}
	s.overlays = []overlay{{button: input.Select, screen: keeper}, {button: input.Start, screen: &page{name: "now playing"}}}
	s.openOverlay(0, t0)
	s.handle(press(input.Start), t0)
	if s.open != 0 || !slices.Equal(keeper.got, []input.Button{input.Start}) {
		t.Fatal("Start left an overlay that keeps it")
	}
}

// tickingPage shows a time that moves on by itself.
type tickingPage struct {
	page
	due time.Time
}

func (p *tickingPage) Due(time.Time) (time.Time, bool) { return p.due, true }

func TestDueIsTheSoonestChange(t *testing.T) {
	tab := &tickingPage{page: page{name: "settings"}, due: t0.Add(800 * time.Millisecond)}
	s := newShell(tab)
	m := &fakeMenu{due: t0.Add(300 * time.Millisecond)}
	s.menu = m
	if d, ok := s.due(t0); !ok || d != tab.due {
		t.Fatalf("due %v, %v with the menu closed; want the screen's", d.Sub(t0), ok)
	}
	m.open = true
	if d, ok := s.due(t0); !ok || d != m.due {
		t.Fatalf("due %v, %v with the menu open; want the menu's", d.Sub(t0), ok)
	}
	a := &App{shell: s, repeat: input.NewRepeater(), fade: newAccentFade(theme.Fallback()), ui: &ui.Context{}}
	if got := a.timeout(t0, t0.Add(100*time.Millisecond)); got != 200*time.Millisecond {
		t.Fatalf("sleep %v, want until the menu's timer changes", got)
	}
}

// lamp is a backlight.
type lamp struct {
	off       bool
	fail      bool
	offs, ons int
}

func (l *lamp) Off() error {
	if l.fail {
		return errors.New("no display driver")
	}
	l.off, l.offs = true, l.offs+1
	return nil
}

func (l *lamp) On() error { l.off, l.ons = false, l.ons+1; return nil }

func darkApp(light Backlight) (*App, *page) {
	tab := &page{name: "home"}
	a := &App{shell: newShell(tab), repeat: input.NewRepeater(), held: map[input.Button]bool{},
		fade: newAccentFade(theme.Fallback()), ui: &ui.Context{},
		opts: Options{Backlight: light, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	return a, tab
}

func release(b input.Button) input.Event { return input.Event{Button: b, Kind: input.Release} }

func TestPowerTurnsTheScreenOffAndOn(t *testing.T) {
	l := &lamp{}
	a, tab := darkApp(l)
	a.dispatch(press(input.Power))
	a.dispatch(release(input.Power))
	if !a.dark || !l.off {
		t.Fatalf("a short press of Power: dark %v, light off %v", a.dark, l.off)
	}
	a.dispatch(press(input.A))
	a.dispatch(press(input.Menu))
	if len(tab.got) > 0 {
		t.Fatalf("buttons reached the screen while it was off: %v", tab.got)
	}
	a.dispatch(press(input.Power))
	a.dispatch(release(input.Power))
	if a.dark || l.off || l.ons != 1 {
		t.Fatalf("a second press: dark %v, light off %v, %d turned on", a.dark, l.off, l.ons)
	}
	a.dispatch(press(input.A))
	if !slices.Equal(tab.got, []input.Button{input.A}) {
		t.Fatal("buttons don't reach the screen once it is on again")
	}
}

// remote records what the buttons heard with the screen off ask of the music.
type remote struct{ got []string }

func (r *remote) Prev()   { r.got = append(r.got, "prev") }
func (r *remote) Next()   { r.got = append(r.got, "next") }
func (r *remote) Toggle() { r.got = append(r.got, "toggle") }

// With the screen off, L1 and R1 skip back and on and A plays or pauses; held,
// they don't repeat, and the rest are ignored.
func TestTheMusicsButtonsWithTheScreenOff(t *testing.T) {
	a, tab := darkApp(&lamp{})
	r := &remote{}
	a.music = r
	a.setDark(true)
	for _, e := range []input.Event{press(input.R1), release(input.R1), press(input.L1), press(input.A),
		{Button: input.R1, Kind: input.Repeat}, press(input.B), press(input.Start), press(input.Down)} {
		a.dispatch(e)
	}
	if !slices.Equal(r.got, []string{"next", "prev", "toggle"}) {
		t.Fatalf("the music was asked for %v", r.got)
	}
	if len(tab.got) > 0 || !a.dark {
		t.Fatalf("with the screen off the screen heard %v; dark %v", tab.got, a.dark)
	}
}

// With the backlight off nothing is drawn, however often something changes;
// the test's context has no canvas, so a frame would panic.
func TestNothingIsDrawnWithTheLightOff(t *testing.T) {
	a, _ := darkApp(&lamp{})
	a.setDark(true)
	for range 3 {
		a.dirty = true
		a.darkFrame()
	}
	if a.dirty {
		t.Fatal("the change is still waiting to be drawn")
	}
}

// A key Boombrick has no use for, as the Brick's volume keys may be, still
// makes a chord of Menu, so NextUI's brightness doesn't open the quick menu.
func TestAnUnusedKeyMakesAChordOfMenu(t *testing.T) {
	a, _ := darkApp(nil)
	m := &fakeMenu{}
	a.shell.menu = m
	a.dispatch(press(input.Menu))
	a.event(&sdl.KeyboardEvent{Type: sdl.KEYDOWN, Keysym: sdl.Keysym{Scancode: 128}})
	a.dispatch(release(input.Menu))
	if m.open {
		t.Fatal("Menu with a key Boombrick doesn't use opened the quick menu")
	}
}

// lateClaimer keeps Menu once its dialog opens.
type lateClaimer struct {
	page
	dialog bool
}

func (c *lateClaimer) Claims(b input.Button) bool { return c.dialog && b == input.Menu }

// A dialog that opens while Menu is down keeps the quick menu from opening
// over it.
func TestADialogOpenedWhileMenuIsDownKeepsIt(t *testing.T) {
	tab := &lateClaimer{page: page{name: "settings"}}
	s := newShell(tab)
	m := &fakeMenu{}
	s.menu = m
	s.handle(press(input.Menu), t0)
	tab.dialog = true
	s.handle(release(input.Menu), t0)
	if m.open {
		t.Fatal("the quick menu opened over the dialog")
	}
}

func TestScreenOffWithoutABacklight(t *testing.T) {
	a, _ := darkApp(&lamp{fail: true})
	a.setDark(true)
	if !a.dark || a.lightOff || !a.dirty {
		t.Fatalf("dark %v, light off %v, dirty %v; want dark, a black frame to draw", a.dark, a.lightOff, a.dirty)
	}
	a.setDark(false) // nothing to turn on
}

func TestQuittingTurnsTheLightBackOn(t *testing.T) {
	l := &lamp{}
	a, _ := darkApp(l)
	a.setDark(true)
	a.lightUp()
	if l.off || l.ons != 1 {
		t.Fatal("the light stays off after quitting with the screen off")
	}
	a.lightUp()
	if l.ons != 1 {
		t.Fatal("the light was turned on twice")
	}
}

func TestHoldingPowerPowersOff(t *testing.T) {
	a, _ := darkApp(nil)
	asked := 0
	a.opts.PowerOff = func() error { asked++; return nil }
	a.dispatch(press(input.Power))
	if a.powerHeld(a.powerAt.Add(powerHold / 2)) {
		t.Fatal("powered off half a hold in")
	}
	if got := a.timeout(a.powerAt, a.powerAt); got != powerHold {
		t.Fatalf("sleep %v while Power is held, want %v", got, powerHold)
	}
	if !a.powerHeld(a.powerAt.Add(powerHold)) || !a.quit || asked != 1 {
		t.Fatalf("held for %v: quit %v, NextUI asked %d times", powerHold, a.quit, asked)
	}
	a.dispatch(release(input.Power))
	if a.dark {
		t.Fatal("letting go after the hold turned the screen off")
	}
}

func TestNothingTicksWhileDark(t *testing.T) {
	a, _ := darkApp(&lamp{})
	a.last.Playing = true
	a.setDark(true)
	if got := a.timeout(t0, t0); got < 10*time.Second {
		t.Fatalf("sleep %v with the screen off and music playing, want until the clock's next minute", got)
	}
}
