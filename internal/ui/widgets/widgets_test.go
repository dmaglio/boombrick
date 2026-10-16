package widgets

import (
	"image"
	"strings"
	"testing"

	"github.com/ujjwal313/boombrick/internal/media"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/text"
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
	FocusGlow(u, card, 12, 1)
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

// With no battery the percentage and glyph go, and "Offline" moves up to the
// right-hand padding.
func TestStatusBarWithoutBattery(t *testing.T) {
	bar := func(battery int) *image.NRGBA {
		u, off := uitest.NewContext(t, theme.DeviceW, int(StatusHeight*theme.Scale))
		u.Status = ui.Status{Time: "21:07", Battery: battery}
		StatusBar(u)
		return off.Image()
	}
	none, some := bar(-1), bar(23)
	// The right-most 40 px hold the battery glyph, or the end of "Offline".
	right := image.Rect(int((theme.ScreenW-28-40)*theme.Scale), 0, int((theme.ScreenW-28)*theme.Scale), none.Bounds().Dy())
	if brightest(some, right) < 200 {
		t.Fatal("the battery glyph should be drawn at the right")
	}
	if b := brightest(none, right); b < 120 || b >= brightest(some, right) {
		t.Fatalf("brightest at the right without a battery = %d: want the gold Offline label there", b)
	}
	if b := brightest(none, image.Rect(int((theme.ScreenW-28)*theme.Scale)+2, 0, theme.DeviceW, none.Bounds().Dy())); b > 40 {
		t.Fatalf("something is drawn in the right-hand padding: %d", b)
	}
	uitest.Golden(t, "status-no-battery", none)
}

// brightest is the highest blue value in r: 242 for paper, 141 for the gold
// Offline label and 20 for the ground.
func brightest(img *image.NRGBA, r image.Rectangle) uint8 {
	var b uint8
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			b = max(b, img.NRGBAAt(x, y).B)
		}
	}
	return b
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

// TestListSheet renders list rows, pills, a toast and the empty state.
func TestListSheet(t *testing.T) {
	u, off := uitest.NewContext(t, theme.DeviceW, theme.DeviceH)
	u.Accent = theme.Derive(35, 0.12)
	DrawBackground(u, StateGround)
	art := SpecFor("Copper Season")
	rows := []Row{
		{Index: "1", Title: "Rust", Duration: "3:42"},
		{Index: "2", Title: "Ember", Duration: "4:05", Playing: true},
		{Art: &art, Title: "Rust", Subtitle: "Odalys · Copper Season", Duration: "3:42"},
		{Art: &art, Round: true, Title: "Odalys", Subtitle: "2 albums · 21 songs"},
		{Grip: true, Art: &art, Title: "Rust", Subtitle: "Odalys", Duration: "3:42"},
	}
	for i, r := range rows {
		k := float32(0)
		if i == 1 {
			k = 1
		}
		box := gfx.Rect{X: 32, Y: 20 + float32(i)*RowHeight, W: theme.ScreenW - 64, H: RowHeight}
		RowGlow(u, box, RowRadius, k)
		ListRow(u, r, box, k)
	}
	x := float32(32)
	PillGlow(u, "play", "Play", x, 440, 1)
	x += Pill(u, "play", "Play", x, 440, 1) + 12
	Pill(u, "shuffle", "", x, 440, 0)
	Toast(u, "Added to queue", 1)
	uitest.Golden(t, "list-sheet", off.Image())
}

func TestEmptyState(t *testing.T) {
	u, off := uitest.NewContext(t, theme.DeviceW, theme.DeviceH)
	DrawBackground(u, StateGround)
	EmptyState(u, "Your library is empty",
		"Copy music to the Music folder on the SD card, or find something on YouTube Music.", "/mnt/SDCARD/Music")
	uitest.Golden(t, "empty-state", off.Image())
}

func TestWrap(t *testing.T) {
	u, _ := uitest.NewContext(t, 64, 64)
	lines := Wrap(u, "one two three four five six seven", text.Body, 120)
	if len(lines) < 2 || strings.Join(lines, " ") != "one two three four five six seven" {
		t.Fatalf("Wrap = %q", lines)
	}
	for _, l := range lines {
		if strings.Contains(l, " ") && u.TextWidth(l, text.Body) > 120 {
			t.Fatalf("line %q is wider than 120", l)
		}
	}
}
