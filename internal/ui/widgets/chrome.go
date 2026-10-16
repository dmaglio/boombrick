// Package widgets draws the components shared by screens (docs/ui.md,
// "Components").
package widgets

import (
	"fmt"
	"image/color"
	"math"

	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
	"github.com/ujjwal313/boombrick/internal/ui/text"
)

// Heights of the fixed bars in design pixels.
const (
	StatusHeight = 44
	TabsHeight   = 72
	HintsHeight  = 56
)

// TabNames are the top-level tabs in order.
var TabNames = [...]string{"Home", "Library", "Spotify", "YouTube", "Downloads"}

// StatusBar draws u.Status across the top of the screen. A negative Battery
// means there is none to show.
func StatusBar(u *ui.Context) {
	s := u.Status
	const h = StatusHeight
	ty := u.CenterY(text.ChromeTnum, 0, h)

	// The right-hand items are measured first: they end 28 px from the edge,
	// and the track title gets whatever space is left of them.
	type item struct {
		w    float32
		draw func(x float32)
	}
	var items []item
	if s.Online {
		items = append(items, item{24, func(x float32) {
			u.Icon("wifi", 24, 2, false, x, (h-24)/2, theme.Paper)
		}})
	} else {
		items = append(items, item{30 + u.TextWidth("Offline", text.Chrome), func(x float32) {
			u.Icon("wifioff", 22, 2, false, x, (h-22)/2, theme.GoldSoft)
			u.Text("Offline", text.Chrome, x+30, ty, theme.GoldSoft)
		}})
	}
	if s.Bluetooth != "" {
		items = append(items, item{28 + u.TextWidth(s.Bluetooth, text.Chrome), func(x float32) {
			u.Icon("headphones", 22, 2, false, x, (h-22)/2, theme.Paper)
			u.Text(s.Bluetooth, text.Chrome, x+28, ty, theme.Paper)
		}})
	}
	if s.Battery >= 0 {
		pct := fmt.Sprintf("%d%%", s.Battery)
		pw := u.TextWidth(pct, text.ChromeTnum)
		items = append(items, item{pw + 8 + 40, func(x float32) {
			u.Text(pct, text.ChromeTnum, x, ty, theme.Paper)
			Battery(u, x+pw+8, (h-18)/2, s.Battery)
		}})
	}

	total := float32(22 * (len(items) - 1))
	for _, it := range items {
		total += it.w
	}
	right := float32(theme.ScreenW) - 28 - total

	x := float32(28)
	x += u.Text(s.Time, text.ChromeTnum, x, ty, theme.Paper)
	if s.NowPlaying != "" {
		x += 14
		u.Icon("music", 18, 2, false, x, (h-18)/2, theme.Ink3)
		x += 26
		u.Text(u.Fit(s.NowPlaying, text.Chrome, right-22-x), text.Chrome, x, ty, theme.Ink3)
	}

	x = right
	for _, it := range items {
		it.draw(x)
		x += it.w + 22
	}
}

// Battery draws the 34 x 18 battery glyph and its terminal, 40 px wide in all.
func Battery(u *ui.Context, x, y float32, percent int) {
	box := gfx.Rect{X: x, Y: y, W: 34, H: 18}
	u.Shape(raster.Shape{W: 34, H: 18, Radius: 5, Kind: raster.Ring, RingOffset: -2, RingWidth: 2}, box, theme.Paper)
	if p := float32(min(max(percent, 0), 100)) / 100; p > 0 {
		u.Fill(gfx.Rect{X: x + 4, Y: y + 4, W: 26 * p, H: 10}, theme.Paper)
	}
	u.Fill(gfx.Rect{X: x + 37, Y: y + 6, W: 3, H: 6}, theme.Paper)
}

// Tabs draws the tab row under the status bar with tab active underlined in
// the accent.
func Tabs(u *ui.Context, active int) {
	const y = StatusHeight
	x := float32(32)
	pillY := y + (TabsHeight-TabPill.Height)/2
	x += KeyGlyph(u, "L1", x, pillY, TabPill, theme.PaperA(0.45)) + 26
	lh := u.LineHeight(text.Tab)
	ty := y + (TabsHeight-lh)/2 - 4
	for i, name := range TabNames {
		col := theme.Ink3
		if i == active {
			col = theme.Paper
		}
		w := u.Text(name, text.Tab, x, ty, col)
		if i == active {
			u.Fill(gfx.Rect{X: x, Y: ty + lh + 4, W: w, H: 3}, u.Accent.Color)
		}
		x += w + 30
	}
	KeyGlyph(u, "R1", x-30+26, pillY, TabPill, theme.PaperA(0.45))
}

// GlyphStyle sizes a button glyph.
type GlyphStyle struct {
	Height float32
	Pad    float32 // horizontal padding inside pills
	Radius float64 // pill corner radius
	Text   text.Style
}

// Glyph styles from docs/ui.md.
var (
	HintRound = GlyphStyle{Height: 34, Text: text.Style{Face: text.LoraSemiBold, Size: 19}}
	HintPill  = GlyphStyle{Height: 34, Pad: 10, Radius: 9, Text: text.Style{Face: text.LoraSemiBold, Size: 17}}
	TabPill   = GlyphStyle{Height: 30, Pad: 9, Radius: 9, Text: text.Style{Face: text.LoraSemiBold, Size: 16}}
)

// arrowIcons draws arrow hints with icons, which the bundled fonts don't cover.
var arrowIcons = map[string][2]string{"◀ ▶": {"left", "right"}, "▲ ▼": {"up", "down"}}

func isFaceButton(key string) bool { return key == "A" || key == "B" || key == "X" || key == "Y" }

// glyphWidth is the width KeyGlyph draws key at.
func glyphWidth(u *ui.Context, key string, gs GlyphStyle) float32 {
	_, isArrow := arrowIcons[key]
	var w float32
	switch {
	case isFaceButton(key):
		w = gs.Height
	case isArrow:
		w = 2*gs.Pad + 36
	default:
		w = max(gs.Height, u.TextWidth(key, gs.Text)+2*gs.Pad)
	}
	return float32(math.Ceil(float64(w)))
}

// KeyGlyph draws a button label: a circle for A, B, X and Y and a pill for
// everything else, outlined 2 px in border. It returns the glyph's width.
func KeyGlyph(u *ui.Context, key string, x, y float32, gs GlyphStyle, border color.NRGBA) float32 {
	h := gs.Height
	w := glyphWidth(u, key, gs)
	radius := gs.Radius
	if isFaceButton(key) {
		radius = float64(h) / 2
	}
	u.Shape(raster.Shape{W: int(w), H: int(h), Radius: radius, Kind: raster.Ring, RingOffset: -2, RingWidth: 2},
		gfx.Rect{X: x, Y: y, W: w, H: h}, border)
	if arrows, ok := arrowIcons[key]; ok {
		iy := y + (h-16)/2
		u.Icon(arrows[0], 16, 3, false, x+gs.Pad, iy, theme.Paper)
		u.Icon(arrows[1], 16, 3, false, x+gs.Pad+20, iy, theme.Paper)
		return w
	}
	tw := u.TextWidth(key, gs.Text)
	u.Text(key, gs.Text, x+(w-tw)/2, u.CenterY(gs.Text, y, h), theme.Paper)
	return w
}

// Hint is one entry in the button hint bar.
type Hint struct {
	Key, Label string
}

// MaxHintsWidth is the room between the hint bar's padding. A screen's hints
// must fit in it (docs/ui.md, "Button hint bar").
const MaxHintsWidth = theme.ScreenW - 2*28

func hintStyle(key string) GlyphStyle {
	if isFaceButton(key) {
		return HintRound
	}
	return HintPill
}

// Spacing in the hint bar, in design pixels.
const (
	hintGap  = 10 // glyph to label
	hintStep = 30 // between hints
)

// HintsWidth measures hints as HintBar lays them out, without its padding.
func HintsWidth(u *ui.Context, hints []Hint) float32 {
	var w float32
	for i, h := range hints {
		if i > 0 {
			w += hintStep
		}
		w += glyphWidth(u, h.Key, hintStyle(h.Key)) + hintGap + u.TextWidth(h.Label, text.Chrome)
	}
	return w
}

// HintBar draws the button hint bar along the bottom of the screen.
func HintBar(u *ui.Context, hints []Hint) {
	const y = theme.ScreenH - HintsHeight
	u.Fill(gfx.Rect{X: 0, Y: y, W: theme.ScreenW, H: HintsHeight}, theme.HintBarBG)
	u.Fill(gfx.Rect{X: 0, Y: y, W: theme.ScreenW, H: 1}, theme.PaperA(0.14))
	ty := u.CenterY(text.Chrome, y, HintsHeight)
	x := float32(28)
	for _, h := range hints {
		gs := hintStyle(h.Key)
		border := theme.PaperA(0.55)
		if h.Key == "A" {
			border = theme.Gold
		}
		x += KeyGlyph(u, h.Key, x, y+(HintsHeight-gs.Height)/2, gs, border) + hintGap
		x += u.Text(h.Label, text.Chrome, x, ty, theme.Paper) + hintStep
	}
}
