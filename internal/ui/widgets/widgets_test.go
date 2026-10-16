package widgets

import (
	"testing"

	"github.com/ujjwal313/boombrick/internal/media"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/uitest"
)

func TestMain(m *testing.M) { uitest.Main(m) }

var designStatus = ui.Status{Time: "9:41", NowPlaying: "Low Tide Radio", Online: true, Bluetooth: "Kite Buds", Battery: 78}

// TestChromeSheet renders every shared component on one frame, laid out like
// the components sheet in the design system.
func TestChromeSheet(t *testing.T) {
	u, off := uitest.NewContext(t, theme.DeviceW, theme.DeviceH)
	u.Accent = theme.Derive(35, 0.12)
	u.Status = designStatus
	DrawBackground(u, TabGround)
	StatusBar(u)
	Tabs(u, 0)

	x := float32(32)
	for _, src := range []media.Source{media.Spotify, media.YouTube, media.Downloaded, media.Local} {
		x += Badge(u, src, x, 140, false) + 12
	}

	specs := []ArtSpec{{35, 0, "S"}, {150, 1, "N"}, {60, 2, "C"}, {265, 3, "A"}, {330, 2, "♥"}}
	for i, spec := range specs {
		Art(u, spec, 140, 12, gfx.Rect{X: 32 + float32(i)*156, Y: 190, W: 140, H: 140})
	}

	Logo(u, 40, 32, 380, theme.Paper, theme.Gold)

	card := gfx.Rect{X: 600, Y: 360, W: 170, H: 170}
	u.PushScale(card.X+card.W/2, card.Y, 1.06)
	FocusRing(u, card, 12, 1, theme.Ground)
	Art(u, ArtSpec{35, 0, "S"}, 170, 12, card)
	u.Pop()

	HintBar(u, []Hint{{Key: "A", Label: "Play"}, {Key: "B", Label: "Back"}, {Key: "X", Label: "Download"},
		{Key: "Y", Label: "Add to queue"}, {Key: "L1/R1", Label: "Tabs"}})

	uitest.Golden(t, "chrome", off.Image())
}

// A title too long for the space left of the right-hand items is cut short
// with an ellipsis instead of running under them.
func TestStatusBarLongTitle(t *testing.T) {
	u, off := uitest.NewContext(t, theme.DeviceW, int(StatusHeight*theme.Scale))
	u.Status = ui.Status{Time: "21:07", NowPlaying: "Paper Lanterns (Extended Session) — The Hollow Pines",
		Online: true, Bluetooth: "Kite Buds", Battery: 78}
	StatusBar(u)
	uitest.Golden(t, "status-long-title", off.Image())
}

func TestStatusBarOffline(t *testing.T) {
	u, off := uitest.NewContext(t, theme.DeviceW, int(StatusHeight*theme.Scale))
	u.Status = ui.Status{Time: "21:07", Battery: 23}
	StatusBar(u)
	uitest.Golden(t, "status-offline", off.Image())
}

func TestSpecForIsStable(t *testing.T) {
	a, b := SpecFor("Morning Ferry"), SpecFor("Morning Ferry")
	if a != b {
		t.Fatalf("SpecFor is not deterministic: %v vs %v", a, b)
	}
	if a.Glyph != "M" || a.Hue < 0 || a.Hue >= 360 || a.Variant < 0 || a.Variant > 3 {
		t.Fatalf("SpecFor = %+v", a)
	}
}
