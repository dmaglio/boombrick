package quickmenu

import (
	"testing"
	"time"

	"github.com/ujjwal313/boombrick/internal/fixture"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/uitest"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

func TestMain(m *testing.M) { uitest.Main(m) }

var t0 = time.Unix(0, 0)

// acts records what the menu asked for.
type acts struct {
	sleep                  player.Sleep
	screenOff, settings, q int
}

func (a *acts) actions() Actions {
	return Actions{
		Sleep:        func() player.Sleep { return a.sleep },
		SetSleep:     func(m player.SleepMode) { a.sleep = player.Sleep{Mode: m} },
		ScreenOff:    func() { a.screenOff++ },
		OpenSettings: func() { a.settings++ },
		Quit:         func() { a.q++ },
	}
}

func press(b input.Button) input.Event { return input.Event{Button: b, Kind: input.Press} }

func TestTheMenu(t *testing.T) {
	a := &acts{sleep: player.Sleep{Mode: player.Sleep30, At: t0.Add(18*time.Minute + 22*time.Second)}}
	m := New(a.actions())
	m.Open(t0)
	u, off := uitest.NewContext(t, theme.DeviceW, theme.DeviceH)
	u.Status = fixture.Status()
	u.BeginFrame(t0.Add(anim.Dialog))
	widgets.DrawBackground(u, widgets.TabGround)
	widgets.StatusBar(u)
	widgets.Tabs(u, 0)
	widgets.HintBar(u, []widgets.Hint{{Key: "L1/R1", Label: "Tabs"}})
	m.Draw(u)
	uitest.Golden(t, "quick-menu", off.Image())
	if got := m.rows[rowSleep].Value; got != "30 min · 18:22 left" {
		t.Fatalf("sleep row says %q", got)
	}
}

func TestMovingAndActing(t *testing.T) {
	a := &acts{}
	m := New(a.actions())
	m.Open(t0)
	if m.Handle(press(input.Up), t0) {
		t.Fatal("Up from the first row moved")
	}
	m.Handle(press(input.Right), t0)
	m.Handle(press(input.Right), t0)
	if a.sleep.Mode != player.Sleep30 {
		t.Fatalf("two Rights set the timer to %v, want 30 min", a.sleep.Mode)
	}
	m.Handle(press(input.Left), t0)
	m.Handle(press(input.A), t0)
	if a.sleep.Mode != player.Sleep30 {
		t.Fatalf("Left then A set the timer to %v, want 30 min", a.sleep.Mode)
	}

	m.Handle(press(input.Down), t0)
	if m.Handle(press(input.Right), t0) {
		t.Fatal("Right on Screen off did something")
	}
	m.Handle(press(input.A), t0)
	if a.screenOff != 1 || m.IsOpen() {
		t.Fatalf("A on Screen off: %d calls, open %v", a.screenOff, m.IsOpen())
	}

	m.Open(t0)
	m.Handle(press(input.Down), t0)
	m.Handle(press(input.Down), t0)
	m.Handle(press(input.A), t0)
	if a.settings != 1 || m.IsOpen() {
		t.Fatalf("A on Settings: %d calls, open %v", a.settings, m.IsOpen())
	}

	m.Open(t0)
	for range 3 {
		m.Handle(press(input.Down), t0)
	}
	m.Handle(press(input.A), t0)
	if a.q != 1 {
		t.Fatal("A on Quit didn't quit")
	}
}

func TestClosing(t *testing.T) {
	m := New((&acts{}).actions())
	shown := t0.Add(anim.Dialog)
	for _, b := range []input.Button{input.B, input.Menu} {
		m.Open(t0)
		if !m.Handle(press(b), shown) || m.IsOpen() {
			t.Fatalf("%v didn't close the menu", b)
		}
		if !m.Visible(shown.Add(anim.Dialog/2)) || m.Visible(shown.Add(anim.Dialog)) {
			t.Fatalf("after %v the menu doesn't fade out", b)
		}
	}
	if m.Handle(press(input.A), t0) {
		t.Fatal("a closed menu took input")
	}
}

func TestDueWhileATimerRuns(t *testing.T) {
	a := &acts{}
	m := New(a.actions())
	m.Open(t0)
	if _, ok := m.Due(t0); ok {
		t.Fatal("due with no timer")
	}
	a.sleep = player.Sleep{Mode: player.Sleep15, At: t0.Add(10*time.Second + 250*time.Millisecond)}
	if due, ok := m.Due(t0); !ok || due != t0.Add(250*time.Millisecond) {
		t.Fatalf("Due = %v, %v", due.Sub(t0), ok)
	}
}
