package widgets

import (
	"fmt"
	"image"
	"image/color"

	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
)

// Background selects a screen ground from docs/ui.md, "Backgrounds".
type Background uint8

const (
	// TabGround glows from the top-left corner.
	TabGround Background = iota
	// NowPlayingGround glows from behind the album art.
	NowPlayingGround
	// FlatGround is plain ground, used by Settings.
	FlatGround
	// StateGround glows from the top centre in the fallback colour, for
	// sign-in, offline and empty-library screens.
	StateGround
)

// DrawBackground fills the screen with kind, coloured by u.Accent. Each
// gradient is rendered once per accent and cached.
func DrawBackground(u *ui.Context, kind Background) {
	if kind == FlatGround {
		u.Canvas.Clear(theme.Ground)
		return
	}
	acc := u.Accent
	key := fmt.Sprintf("bg:%d:%v:%.1f", kind, acc.Glow, acc.Hue)
	u.Image(key, func() *image.NRGBA {
		const w, h = theme.DeviceW, theme.DeviceH
		ground := theme.OklabOf(theme.Ground)
		if kind == NowPlayingGround {
			return raster.RadialGradient(w, h, 0.22*w, 0.45*h, 0.9*w, 1.2*h, []raster.Stop{
				{Pos: 0, Color: theme.OKLCH{L: 0.36, C: 0.09, H: acc.Hue}.Oklab()},
				{Pos: 0.45, Color: theme.OKLCH{L: 0.24, C: 0.05, H: acc.Hue}.Oklab()},
				{Pos: 0.85, Color: ground},
			})
		}
		if kind == StateGround {
			return raster.RadialGradient(w, h, 0.5*w, 0, 1.3*w, h, []raster.Stop{
				{Pos: 0, Color: theme.OklabOf(theme.FallbackGlow)},
				{Pos: 0.62, Color: ground},
			})
		}
		return raster.RadialGradient(w, h, 0, 0, 1.3*w, h, []raster.Stop{
			{Pos: 0, Color: theme.OklabOf(acc.Glow)},
			{Pos: 0.62, Color: ground},
		})
	}, gfx.Rect{X: 0, Y: 0, W: theme.ScreenW, H: theme.ScreenH}, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
}
