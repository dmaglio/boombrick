package widgets

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/icons"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
	"github.com/ujjwal313/boombrick/internal/ui/text"
)

// ArtSpec describes generated placeholder art.
type ArtSpec struct {
	Hue     float64
	Variant int
	Glyph   string
}

// SpecFor derives stable placeholder art for an album or playlist name.
func SpecFor(name string) ArtSpec {
	h := fnv.New32a()
	h.Write([]byte(name))
	v := h.Sum32()
	r, _ := utf8.DecodeRuneInString(name)
	return ArtSpec{Hue: float64(v % 360), Variant: int(v / 360 % 4), Glyph: strings.ToUpper(string(r))}
}

// Art draws placeholder art for spec, size design pixels square, into box.
func Art(u *ui.Context, spec ArtSpec, size int, radius float64, box gfx.Rect) {
	px := u.Px(float32(size))
	key := fmt.Sprintf("art:%g:%d:%s:%d:%g", spec.Hue, spec.Variant, spec.Glyph, px, radius)
	u.Image(key, func() *image.NRGBA {
		var glyph *image.NRGBA
		gx, gy := 0, 0
		if spec.Variant%4 == 2 && spec.Glyph != "" {
			glyph, gx, gy = artGlyph(u, spec.Glyph, px)
		}
		return raster.Placeholder(px, radius*float64(u.Scale), spec.Hue, spec.Variant, glyph, gx, gy)
	}, box, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
}

// artGlyph renders the letter for variant 2: Cormorant Italic at 90 % of the
// art size, 10 % from the left, in a line box that ends 6 % below the art.
// Liked Songs uses a heart icon instead of ♥, which the fonts lack. size is in
// device pixels: this calls the text engine directly, which works in whatever
// pixels the style gives it.
func artGlyph(u *ui.Context, glyph string, size int) (*image.NRGBA, int, int) {
	s := float64(size)
	if glyph == "♥" {
		hs := int(0.45 * s)
		img, err := icons.Rasterize("heart", hs, 0, true)
		if err != nil {
			return nil, 0, 0
		}
		return img, int(0.12 * s), size - hs - int(0.08*s)
	}
	fs := int(math.Round(0.9 * s))
	st := text.Style{Face: text.CormorantItalic, Size: float32(fs)}
	line := u.Fonts.Shape(glyph, st)
	img, pad := u.Fonts.Rasterize(line, st)
	top := 1.06*s - float64(fs) + (float64(fs)-float64(line.Height()))/2
	return img, int(0.1*s) - pad, int(math.Round(top))
}
