package home

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/ujjwal313/boombrick/internal/fixture"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/uitest"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

func TestMain(m *testing.M) { uitest.Main(m) }

// Both of Home's hint sets fit the bar (docs/ui.md, "Button hint bar").
func TestHintsFit(t *testing.T) {
	u, _ := uitest.NewContext(t, 64, 64)
	h := New(fixture.Home())
	for _, row := range []int{0, 2} {
		h.row = row
		if w := widgets.HintsWidth(u, h.hints()); w > widgets.MaxHintsWidth {
			t.Errorf("row %d hints are %.1f wide, more than %.1f", row, w, widgets.MaxHintsWidth)
		}
	}
}

var t0 = time.Unix(1000, 0)

func press(b input.Button) input.Event { return input.Event{Button: b, Kind: input.Press} }

func render(t *testing.T, h *Home, now time.Time) *image.NRGBA {
	t.Helper()
	u, off := uitest.NewContext(t, theme.DeviceW, theme.DeviceH)
	u.Accent = theme.Derive(fixture.PlayingHue, fixture.PlayingChroma)
	u.Status = fixture.Status()
	u.Now = now
	h.Draw(u)
	return off.Image()
}

// Frame 01a: focus on the first Continue listening card.
func TestFirstCardFocused(t *testing.T) {
	uitest.Golden(t, "home-01a", render(t, New(fixture.Home()), t0))
}

// Frame 01b: D-pad right moves the ring and scale to the next card in 120 ms.
func TestFocusMovesRight(t *testing.T) {
	h := New(fixture.Home())
	render(t, h, t0)
	if !h.Handle(press(input.Right), t0) {
		t.Fatal("Right should move focus")
	}
	if !h.Animating(t0.Add(anim.Focus / 2)) {
		t.Fatal("focus should be animating halfway through")
	}
	end := t0.Add(anim.Focus)
	if h.Animating(end) {
		t.Fatal("focus animation should end after anim.Focus")
	}
	uitest.Golden(t, "home-01b", render(t, h, end))
}

// Frame 01c: after moving down twice the focused row sits under the tabs.
func TestFocusedRowScrollsUnderTabs(t *testing.T) {
	h := New(fixture.Home())
	render(t, h, t0)
	for _, b := range []input.Button{input.Right, input.Down, input.Down} {
		h.Handle(press(b), t0)
	}
	if h.row != 2 || h.col != 1 {
		t.Fatalf("focus = row %d col %d, want 2, 1", h.row, h.col)
	}
	if got := h.scroll.Target(); got <= 0 || got > h.tops[2] {
		t.Fatalf("scroll target = %v, want (0, %v]", got, h.tops[2])
	}
	uitest.Golden(t, "home-01c", render(t, h, t0.Add(anim.Focus)))
}

// The last row snaps under the tabs like the others, so its focused tile,
// scaled and ringed, is never clipped by the hint bar.
func TestLastRowScrollsUnderTabs(t *testing.T) {
	h := New(fixture.Home())
	render(t, h, t0)
	last := len(h.rows) - 1
	for i := 0; i < last; i++ {
		h.Handle(press(input.Down), t0)
	}
	if got, want := h.scroll.Target(), h.tops[last]; got != want {
		t.Fatalf("scroll target = %v, want %v (heading under the tabs)", got, want)
	}
}

func TestFocusStaysInBounds(t *testing.T) {
	h := New(fixture.Home())
	if h.Handle(press(input.Left), t0) || h.Handle(press(input.Up), t0) {
		t.Fatal("moving off the first card should do nothing")
	}
	for i := 0; i < 5; i++ {
		h.Handle(press(input.Right), t0)
	}
	if h.col != 3 {
		t.Fatalf("col = %d, want 3 (four cards)", h.col)
	}
	h.Handle(press(input.Down), t0)  // Downloads has five tiles
	h.Handle(press(input.Right), t0) // to the fifth
	h.Handle(press(input.Down), t0)  // Spotify playlists has four cards
	if h.row != 2 || h.col != 3 {
		t.Fatalf("focus = %d,%d, want 2,3 (column clamped)", h.row, h.col)
	}
}

// A row scrolls sideways just enough to bring the focused card between the
// side paddings; the first cards don't move it.
func TestRowScrollsSideways(t *testing.T) {
	h := New(fixture.Home())
	h.Handle(press(input.Right), t0)
	h.Handle(press(input.Right), t0)
	if got := h.offsets[0].Target(); got != 0 {
		t.Fatalf("offset on the third card = %v, want 0", got)
	}
	h.Handle(press(input.Right), t0)
	want := float64(3*(cardSize+cardGap)+cardSize) - (theme.ScreenW - 2*side)
	if got := h.offsets[0].Target(); math.Abs(got-want) > 1e-6 {
		t.Fatalf("offset on the fourth card = %v, want %v", got, want)
	}
	if !h.Animating(t0.Add(anim.Focus / 2)) {
		t.Fatal("the row should be scrolling halfway through")
	}
}

// Rows keep their sideways position when focus leaves and comes back.
func TestRowsRememberTheirOffset(t *testing.T) {
	h := New(fixture.Home())
	for i := 0; i < 3; i++ {
		h.Handle(press(input.Right), t0)
	}
	want := h.offsets[0].Target()
	h.Handle(press(input.Down), t0)
	h.Handle(press(input.Up), t0)
	if h.row != 0 || h.col != 3 {
		t.Fatalf("focus = %d,%d, want 0,3", h.row, h.col)
	}
	if got := h.offsets[0].Target(); got != want {
		t.Fatalf("offset after leaving and returning = %v, want %v", got, want)
	}
}

// The fourth Continue listening card, scrolled into view.
func TestScrolledRowGolden(t *testing.T) {
	h := New(fixture.Home())
	render(t, h, t0)
	for i := 0; i < 3; i++ {
		h.Handle(press(input.Right), t0)
	}
	uitest.Golden(t, "home-scrolled", render(t, h, t0.Add(anim.Focus)))
}

func TestReleaseDoesNothing(t *testing.T) {
	h := New(fixture.Home())
	if h.Handle(input.Event{Button: input.Right, Kind: input.Release}, t0) {
		t.Fatal("a release should not move focus")
	}
}
