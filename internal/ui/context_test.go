package ui

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/ujjwal313/boombrick/assets"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/text"
)

func TestMain(m *testing.M) {
	runtime.LockOSThread()
	os.Exit(m.Run())
}

func newContext(t *testing.T, scale float32) (*Context, *gfx.Offscreen) {
	t.Helper()
	off, err := gfx.NewOffscreen(200, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(off.Close)
	fonts, err := text.NewEngine(assets.Fonts())
	if err != nil {
		t.Fatal(err)
	}
	return NewContext(off.Canvas, fonts, scale), off
}

func TestScaleMapsToDevicePixels(t *testing.T) {
	u := Context{Scale: 1.25}
	u.PushScale(0, 0, 2)
	if got, want := u.Apply(gfx.Rect{X: 4, Y: 8, W: 8, H: 4}), (gfx.Rect{X: 10, Y: 20, W: 20, H: 10}); got != want {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

func TestScaledTextIsMeasuredInDesignPixels(t *testing.T) {
	plain, _ := newContext(t, 1)
	scaled, _ := newContext(t, 1.25)
	w1 := plain.TextWidth("Continue listening", text.Heading)
	w2 := scaled.TextWidth("Continue listening", text.Heading)
	if d := w2 - w1; d < -2 || d > 2 {
		t.Fatalf("width at scale 1.25 = %v design px, at scale 1 = %v", w2, w1)
	}
	scaled.Text("Home", text.Body, 0, 0, theme.Paper)
	e, ok := scaled.texts.Get(textKey{"Home", text.Body})
	if !ok {
		t.Fatal("text was not cached")
	}
	if least := plain.TextWidth("Home", text.Body) * 1.25; float32(e.tex.W) < least {
		t.Fatalf("texture is %d px wide, want at least %v: text must be rasterised at device size", e.tex.W, least)
	}
}

func TestIconsRasteriseAtDeviceSize(t *testing.T) {
	u, _ := newContext(t, 1.25)
	u.Icon("play", 16, 2, true, 0, 0, theme.Paper)
	if _, ok := u.icons[iconKey{"play", 20, 2, true}]; !ok {
		t.Fatal("a 16 px icon at scale 1.25 should be rasterised at 20 px")
	}
}

func TestTextDrawsAndMeasures(t *testing.T) {
	u, off := newContext(t, 1)
	off.Clear(theme.Ground)
	w := u.Text("Home", text.Body, 10, 10, theme.Paper)
	if w <= 0 || w != u.TextWidth("Home", text.Body) {
		t.Fatalf("width = %v, TextWidth = %v", w, u.TextWidth("Home", text.Body))
	}
	img := off.Image()
	lit := 0
	for y := 10; y < 10+int(u.LineHeight(text.Body)); y++ {
		for x := 10; x < 10+int(w); x++ {
			if img.NRGBAAt(x, y) != theme.Ground {
				lit++
			}
		}
	}
	if lit == 0 {
		t.Fatal("no text was drawn")
	}
}

func TestApplyNestsTransforms(t *testing.T) {
	u := Context{Scale: 1}
	r := gfx.Rect{X: 20, Y: 10, W: 10, H: 10}
	u.PushScale(100, 0, 2)
	u.PushScale(10, 10, 0.5)
	// Inner first: (15, 10, 5x5); then outer: (-70, 20, 10x10).
	if got, want := u.Apply(r), (gfx.Rect{X: -70, Y: 20, W: 10, H: 10}); got != want {
		t.Errorf("Apply = %v, want %v", got, want)
	}
	u.Pop()
	u.Pop()
	if got := u.Apply(r); got != r {
		t.Errorf("after Pop, Apply = %v, want identity", got)
	}
}

func TestFitIsCached(t *testing.T) {
	u, _ := newContext(t, 1)
	got := u.Fit("Paper Lanterns — The Hollow Pines", text.Body, 120)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("Fit = %q", got)
	}
	if len(u.fits) != 1 {
		t.Fatalf("fits cache has %d entries", len(u.fits))
	}
}
