package nowplaying

import (
	"testing"
	"time"

	"github.com/ujjwal313/boombrick/internal/fixture"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/uitest"
)

func TestMain(m *testing.M) { uitest.Main(m) }

type fake struct {
	st                             player.State
	toggles, nexts, prevs, repeats int
	shuffles                       []bool
}

func (f *fake) State() player.State { return f.st }
func (f *fake) Toggle()             { f.toggles++ }
func (f *fake) Next()               { f.nexts++ }
func (f *fake) Prev()               { f.prevs++ }
func (f *fake) SetShuffle(on bool)  { f.shuffles = append(f.shuffles, on) }
func (f *fake) CycleRepeat()        { f.repeats++ }

var t0 = time.Unix(1000, 0)

func press(b input.Button) input.Event { return input.Event{Button: b, Kind: input.Press} }

func playing() *fake {
	return &fake{st: player.State{
		Queued: true, Playing: true, Count: 9, Index: 2,
		Track:   player.Track{Path: "a.flac", Title: "Low Tide Radio", Artist: "Marisol Vega", Album: "Salt & Static"},
		Elapsed: 93 * time.Second, Duration: 222 * time.Second, Rate: 44100, Format: "FLAC",
		Shuffle: true, Repeat: player.RepeatAll,
	}}
}

func TestGolden(t *testing.T) {
	u, off := uitest.NewContext(t, theme.DeviceW, theme.DeviceH)
	u.Accent = theme.Derive(fixture.PlayingHue, fixture.PlayingChroma)
	u.Status = fixture.Status()
	u.Now = t0
	New(playing()).Draw(u)
	uitest.Golden(t, "nowplaying", off.Image())
}

func TestFocusMovesAlongTheControls(t *testing.T) {
	n := New(playing())
	if !n.Handle(press(input.Right), t0) || !n.Handle(press(input.Right), t0) {
		t.Fatal("Right should move focus from play/pause to repeat")
	}
	if n.focus != repeatCtl || n.Handle(press(input.Right), t0) {
		t.Fatalf("focus = %d; Right past repeat should do nothing", n.focus)
	}
	if !n.Animating(t0.Add(time.Millisecond)) {
		t.Fatal("focus should be animating")
	}
}

func TestAnimatingSettles(t *testing.T) {
	n := New(playing())
	if n.Animating(t0) {
		t.Fatal("a fresh screen should not be animating")
	}
	n.Handle(press(input.Right), t0)
	if n.Animating(t0.Add(anim.Focus + time.Millisecond)) {
		t.Fatal("focus animation should be done once anim.Focus has passed")
	}
}

func TestAActivatesTheFocusedControl(t *testing.T) {
	f := playing()
	n := New(f)
	n.Handle(press(input.A), t0) // play/pause
	n.Handle(press(input.Left), t0)
	n.Handle(press(input.A), t0) // previous
	n.Handle(press(input.Left), t0)
	n.Handle(press(input.A), t0) // shuffle off
	for i := 0; i < 4; i++ {
		n.Handle(press(input.Right), t0)
	}
	n.Handle(press(input.A), t0) // repeat
	if f.toggles != 1 || f.prevs != 1 || len(f.shuffles) != 1 || f.shuffles[0] || f.repeats != 1 {
		t.Fatalf("toggles %d, prevs %d, shuffles %v, repeats %d", f.toggles, f.prevs, f.shuffles, f.repeats)
	}
}

func TestShouldersChangeTrack(t *testing.T) {
	f := playing()
	n := New(f)
	n.Handle(press(input.L1), t0)
	n.Handle(press(input.R1), t0)
	n.Handle(press(input.R1), t0)
	if f.prevs != 1 || f.nexts != 2 {
		t.Fatalf("prevs %d, nexts %d", f.prevs, f.nexts)
	}
}

func TestHintFollowsFocusAndState(t *testing.T) {
	f := playing()
	n := New(f)
	if h := n.hints(f.st); h[0].Label != "Pause" {
		t.Fatalf("playing: A hint %q, want Pause", h[0].Label)
	}
	f.st.Playing = false
	if h := n.hints(f.st); h[0].Label != "Play" {
		t.Fatalf("paused: A hint %q, want Play", h[0].Label)
	}
	n.Handle(press(input.Right), t0)
	if h := n.hints(f.st); h[0].Label != "Next" {
		t.Fatalf("on next: A hint %q, want Next", h[0].Label)
	}
}
