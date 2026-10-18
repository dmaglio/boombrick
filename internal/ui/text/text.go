// Package text shapes and rasterises strings with Boombrick's bundled fonts.
package text

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"math"
	"strings"

	"github.com/go-text/render"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"github.com/ujjwal313/boombrick/internal/ui/raster"
)

// Face selects one of the bundled font files.
type Face uint8

const (
	Cormorant Face = iota
	CormorantSemiBold
	CormorantItalic
	Lora
	LoraSemiBold
	LoraItalic
	faceCount
)

var files = [faceCount]string{
	Cormorant:         "CormorantGaramond-Regular.ttf",
	CormorantSemiBold: "CormorantGaramond-SemiBold.ttf",
	CormorantItalic:   "CormorantGaramond-Italic.ttf",
	Lora:              "Lora-Regular.ttf",
	LoraSemiBold:      "Lora-SemiBold.ttf",
	LoraItalic:        "Lora-Italic.ttf",
}

// Style is a face at a pixel size, which may be fractional. Tabular turns on
// the font's tabular figures.
type Style struct {
	Face    Face
	Size    float32
	Tabular bool
}

// Tnum returns st with tabular figures.
func (st Style) Tnum() Style {
	st.Tabular = true
	return st
}

// Named styles.
var (
	Display    = Style{Face: Cormorant, Size: 56}
	Title      = Style{Face: CormorantSemiBold, Size: 44}
	Heading    = Style{Face: CormorantSemiBold, Size: 34}
	Tab        = Style{Face: CormorantSemiBold, Size: 30}
	Body       = Style{Face: Lora, Size: 28}
	Meta       = Style{Face: Lora, Size: 22}
	Chrome     = Style{Face: Lora, Size: 22}
	ChromeTnum = Style{Face: Lora, Size: 22, Tabular: true}
	Badge      = Style{Face: Lora, Size: 20}
)

const ellipsis = "…"

var tnum = []shaping.FontFeature{{Tag: opentype.MustNewTag("tnum"), Value: 1}}

// Engine owns the parsed fonts. It is not safe for concurrent use; the UI
// thread owns it.
type Engine struct {
	faces   [faceCount]*font.Face
	shaper  shaping.HarfbuzzShaper
	metrics map[Style][2]int
}

// NewEngine loads the bundled fonts from fonts.
func NewEngine(fonts fs.FS) (*Engine, error) {
	e := &Engine{metrics: map[Style][2]int{}}
	for i, name := range files {
		data, err := fs.ReadFile(fonts, name)
		if err != nil {
			return nil, err
		}
		face, err := font.ParseTTF(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		e.faces[i] = face
	}
	return e, nil
}

// Line is a shaped line of text.
type Line struct {
	out     shaping.Output
	Width   int // advance in pixels
	Ascent  int
	Descent int
}

// Height is the line box: ascent plus descent.
func (l Line) Height() int { return l.Ascent + l.Descent }

// Shape lays out s on one line.
func (e *Engine) Shape(s string, st Style) Line {
	runes := []rune(s)
	size := fixed.Int26_6(math.Round(float64(st.Size) * 64))
	in := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Direction: di.DirectionLTR,
		Face:      e.faces[st.Face],
		// The shaper rounds its size up to a whole pixel; the output is
		// scaled back to size below.
		Size:     fixed.I(size.Ceil()),
		Script:   language.Latin,
		Language: language.NewLanguage("en"),
	}
	if st.Tabular {
		in.FontFeatures = tnum
	}
	out := e.shaper.Shape(in)
	if in.Size != size {
		rescale(&out, float64(size)/float64(in.Size))
		out.Size = size
	}
	return Line{
		out:     out,
		Width:   out.Advance.Ceil(),
		Ascent:  out.LineBounds.Ascent.Ceil(),
		Descent: (-out.LineBounds.Descent).Ceil(),
	}
}

// rescale multiplies every metric of out by f.
func rescale(out *shaping.Output, f float64) {
	sc := func(v fixed.Int26_6) fixed.Int26_6 { return fixed.Int26_6(math.Round(float64(v) * f)) }
	for i := range out.Glyphs {
		g := &out.Glyphs[i]
		g.Width, g.Height = sc(g.Width), sc(g.Height)
		g.XBearing, g.YBearing = sc(g.XBearing), sc(g.YBearing)
		g.Advance, g.XAdvance, g.YAdvance = sc(g.Advance), sc(g.XAdvance), sc(g.YAdvance)
		g.XOffset, g.YOffset = sc(g.XOffset), sc(g.YOffset)
	}
	out.LineBounds = shaping.Bounds{Ascent: sc(out.LineBounds.Ascent), Descent: sc(out.LineBounds.Descent), Gap: sc(out.LineBounds.Gap)}
	out.RecalculateAll()
}

// Metrics returns the ascent and descent of st's line box.
func (e *Engine) Metrics(st Style) (ascent, descent int) {
	st.Tabular = false
	if m, ok := e.metrics[st]; ok {
		return m[0], m[1]
	}
	l := e.Shape(" ", st)
	e.metrics[st] = [2]int{l.Ascent, l.Descent}
	return l.Ascent, l.Descent
}

// Fit returns s if it fits in maxWidth, otherwise the longest prefix that fits
// with an ellipsis appended.
func (e *Engine) Fit(s string, st Style, maxWidth int) string {
	if e.Shape(s, st).Width <= maxWidth {
		return s
	}
	runes := []rune(s)
	cut := func(n int) string { return strings.TrimRight(string(runes[:n]), " ") + ellipsis }
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if e.Shape(cut(mid), st).Width <= maxWidth {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return cut(lo)
}

// Pad is the horizontal padding Rasterize adds on each side so italic
// overhangs aren't clipped.
func Pad(st Style) int { return int(st.Size)/8 + 1 }

// Rasterize draws l as a white mask Pad(st) px wider than its advance on each
// side. It returns the mask and that padding; draw the mask Pad px to the left
// of the text position.
func (e *Engine) Rasterize(l Line, st Style) (*image.NRGBA, int) {
	pad := Pad(st)
	h := l.Height()
	if h < 1 {
		h = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, l.Width+2*pad, h))
	r := render.Renderer{FontSize: st.Size, PixScale: 1, Color: color.White}
	r.DrawShapedRunAt(l.out, dst, pad, l.Ascent)
	return raster.WhiteMask(dst), pad
}
