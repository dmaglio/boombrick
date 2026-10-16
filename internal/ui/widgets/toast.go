package widgets

import (
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
	"github.com/ujjwal313/boombrick/internal/ui/text"
)

var toastText = text.Style{Face: text.Lora, Size: 24}

// Toast draws msg centred 16 px above the hint bar on the raised colour,
// padding 14 22 and radius 14 (docs/ui.md, "Toast"). alpha fades it.
func Toast(u *ui.Context, msg string, alpha float32) {
	w := u.TextWidth(msg, toastText) + 44
	h := u.LineHeight(toastText) + 28
	box := gfx.Rect{X: (theme.ScreenW - w) / 2, Y: theme.ScreenH - HintsHeight - 16 - h, W: w, H: h}
	u.Shape(raster.Shape{W: int(w), H: int(h), Radius: 14, Kind: raster.Fill}, box, fade(theme.Raised, alpha))
	u.Text(msg, toastText, box.X+22, box.Y+14, fade(theme.Paper, alpha))
}
