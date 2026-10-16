package text

import (
	"strings"
	"testing"

	"github.com/ujjwal313/boombrick/assets"
)

func engine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewEngine(assets.Fonts())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestTabularFiguresShareWidth(t *testing.T) {
	e := engine(t)
	st := Body.Tnum()
	if a, b := e.Shape("1111", st).Width, e.Shape("8888", st).Width; a != b {
		t.Fatalf("tabular widths differ: 1111=%d 8888=%d", a, b)
	}
}

func TestFitAddsEllipsis(t *testing.T) {
	e := engine(t)
	long := "Paper Lanterns — The Hollow Pines — Extended Session"
	got := e.Fit(long, Body, 200)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("Fit = %q, want an ellipsis", got)
	}
	if w := e.Shape(got, Body).Width; w > 200 {
		t.Fatalf("fitted width %d > 200", w)
	}
	if got := e.Fit("Home", Body, 200); got != "Home" {
		t.Fatalf("short text changed to %q", got)
	}
}

func TestMetricsMatchShaping(t *testing.T) {
	e := engine(t)
	a, d := e.Metrics(Heading)
	l := e.Shape("Continue listening", Heading)
	if a != l.Ascent || d != l.Descent {
		t.Fatalf("Metrics = %d/%d, Shape = %d/%d", a, d, l.Ascent, l.Descent)
	}
	if a <= 0 || d <= 0 {
		t.Fatalf("implausible metrics %d/%d", a, d)
	}
}

func TestFractionalSize(t *testing.T) {
	e := engine(t)
	at := func(size float32) int { return e.Shape("Downloads", Style{Face: Lora, Size: size}).Width }
	if w := at(27.5); w <= at(27) || w >= at(28) {
		t.Fatalf("width at 27.5 = %d, want between %d and %d", w, at(27), at(28))
	}
}

func TestRasterizeCoversText(t *testing.T) {
	e := engine(t)
	line := e.Shape("Boombrick", Title)
	img, pad := e.Rasterize(line, Title)
	if img.Bounds().Dy() != line.Height() {
		t.Fatalf("height = %d, want %d", img.Bounds().Dy(), line.Height())
	}
	right := -1
	for x := img.Bounds().Dx() - 1; x >= 0 && right < 0; x-- {
		for y := 0; y < img.Bounds().Dy(); y++ {
			if img.NRGBAAt(x, y).A > 0 {
				right = x
				break
			}
		}
	}
	ink := right - pad
	if ink < line.Width-int(Title.Size) || ink > line.Width+pad {
		t.Fatalf("ink ends at %d, advance is %d", ink, line.Width)
	}
}
