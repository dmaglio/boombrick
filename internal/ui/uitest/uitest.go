// Package uitest draws UI into memory for tests and compares frames with
// approved images in testdata. Set BOOMBRICK_UPDATE_GOLDEN=1 to rewrite them.
package uitest

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/ujjwal313/boombrick/assets"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/text"
)

// Main runs a package's tests on the main OS thread, as SDL requires.
func Main(m *testing.M) {
	runtime.LockOSThread()
	os.Exit(m.Run())
}

// NewContext returns a context drawing into a w x h device pixel offscreen
// canvas cleared to the ground colour.
func NewContext(t *testing.T, w, h int) (*ui.Context, *gfx.Offscreen) {
	t.Helper()
	// The app scales textures with linear filtering; frames must match.
	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "1")
	off, err := gfx.NewOffscreen(w, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(off.Close)
	fonts, err := text.NewEngine(assets.Fonts())
	if err != nil {
		t.Fatal(err)
	}
	u := ui.NewContext(off.Canvas, fonts, theme.Scale)
	u.Now = time.Unix(0, 0)
	off.Clear(theme.Ground)
	return u, off
}

// Golden compares got with testdata/<name>.png. If more than 0.5 % of pixels
// differ by more than 2 in any channel, the test fails and the frame is
// written to testdata/<name>.actual.png for inspection.
func Golden(t *testing.T, name string, got *image.NRGBA) {
	t.Helper()
	path := filepath.Join("testdata", name+".png")
	if os.Getenv("BOOMBRICK_UPDATE_GOLDEN") != "" {
		if err := writePNG(path, got); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
		return
	}
	want, err := readPNG(path)
	if err != nil {
		t.Fatalf("%v (run with BOOMBRICK_UPDATE_GOLDEN=1 to create it)", err)
	}
	if want.Bounds() != got.Bounds() {
		t.Fatalf("%s: size %v, want %v", name, got.Bounds(), want.Bounds())
	}
	bad := 0
	for i := 0; i < len(got.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			d := int(got.Pix[i+c]) - int(want.Pix[i+c])
			if d > 2 || d < -2 {
				bad++
				break
			}
		}
	}
	if total := len(got.Pix) / 4; bad*1000 > total*5 {
		actual := filepath.Join("testdata", name+".actual.png")
		if err := writePNG(actual, got); err != nil {
			t.Error(err)
		}
		t.Fatalf("%s: %d of %d pixels differ; wrote %s", name, bad, total, actual)
	}
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readPNG(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out, nil
}
