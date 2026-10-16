package widgets

import (
	"image/color"

	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
)

// FocusRing draws the card and button focus treatment around box: a glow
// (44 px blur, 8 px spread, accent at 38 %), a 4 px accent ring and a 3 px gap
// in gap colour between the ring and the element. strength fades it while
// focus moves. Draw it before the element itself.
func FocusRing(u *ui.Context, box gfx.Rect, radius float64, strength float32, gap color.NRGBA) {
	if strength <= 0 {
		return
	}
	w, h := int(box.W), int(box.H)
	acc := u.Accent.Color
	u.Shape(raster.Shape{W: w, H: h, Radius: radius, Kind: raster.Glow, GlowSpread: 8, GlowBlur: 44}, box, fade(acc, 0.38*strength))
	u.Shape(raster.Shape{W: w, H: h, Radius: radius, Kind: raster.Ring, RingWidth: 7}, box, fade(acc, strength))
	u.Shape(raster.Shape{W: w, H: h, Radius: radius, Kind: raster.Ring, RingWidth: 3}, box, fade(gap, strength))
}

// RowFocus draws the list-row focus treatment: accent tint, a 4 px ring and a
// 30 px glow. Draw it before the row's content.
func RowFocus(u *ui.Context, box gfx.Rect, radius float64, strength float32) {
	if strength <= 0 {
		return
	}
	w, h := int(box.W), int(box.H)
	acc := u.Accent.Color
	u.Shape(raster.Shape{W: w, H: h, Radius: radius, Kind: raster.Glow, GlowSpread: 2, GlowBlur: 30}, box, fade(acc, 0.30*strength))
	u.Shape(raster.Shape{W: w, H: h, Radius: radius, Kind: raster.Fill}, box, fade(acc, 0.14*strength))
	u.Shape(raster.Shape{W: w, H: h, Radius: radius, Kind: raster.Ring, RingWidth: 4}, box, fade(acc, strength))
}
