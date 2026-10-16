package widgets

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/ujjwal313/boombrick/assets"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
	"github.com/ujjwal313/boombrick/internal/ui/text"
)

// LogoMark draws the boombox mark at em px per em: 1.5 em wide and 1.27 em
// tall including the handle.
func LogoMark(u *ui.Context, em, x, y float32, col color.NRGBA) {
	w, h := 1.5*em, 1.27*em
	pw, ph := u.Px(w), u.Px(h)
	u.Image(fmt.Sprintf("logo:%dx%d", pw, ph), func() *image.NRGBA {
		img, err := raster.SVG(assets.Logo(), pw, ph)
		if err != nil {
			return nil
		}
		return img
	}, gfx.Rect{X: x, Y: y, W: float32(pw) / u.Scale, H: float32(ph) / u.Scale}, col)
}

// Logo draws the mark and the wordmark: "Boom" upright in ink, "brick" in
// italic gold.
func Logo(u *ui.Context, em, x, y float32, ink, gold color.NRGBA) {
	LogoMark(u, em, x, y, gold)
	size := float32(math.Round(float64(1.12 * em)))
	boom := text.Style{Face: text.CormorantSemiBold, Size: size}
	brick := text.Style{Face: text.CormorantItalic, Size: size}
	tx := x + 1.92*em
	ty := y + 0.63*em - u.LineHeight(boom)/2
	w := u.Text("Boom", boom, tx, ty, ink)
	u.Text("brick", brick, tx+w, ty, gold)
}
