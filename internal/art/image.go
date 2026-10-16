// Package art finds album art, scales it once to the sizes the screens draw
// and keeps the copies on disk, with each album's colour and the Lyrics
// screen's blurred backdrop.
package art

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg" // decoders for Decode
	_ "image/png"
	"math"

	_ "golang.org/x/image/webp"

	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
)

// maxDecodeBytes is the memory one decode may take: half the device's 96 MB
// heap, since the cache decodes one picture at a time. A baseline JPEG takes
// 1.5 bytes a pixel while decoding if its colour is stored at quarter
// resolution (4:2:0) and 3 if not (4:4:4). A progressive one takes 5 times
// that, 7.5 and 15, because Go keeps every coefficient of the image as an
// int32. A PNG decodes to 4 bytes a pixel, or 8 at 16 bits a channel, and WebP to 4.
const maxDecodeBytes = 48 << 20

// ErrTooBig is returned for a picture too large to decode on the device.
var ErrTooBig = errors.New("art: picture too large")

// Decode decodes a JPEG, PNG or WebP picture, refusing one too large to
// decode in the memory the device has.
func Decode(data []byte) (image.Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	perPixel := 4.0
	switch format {
	case "jpeg":
		var ok bool
		if perPixel, ok = jpegCost(data); !ok {
			return nil, ErrTooBig
		}
	case "png":
		perPixel = pngCost(data)
	}
	if tooBig(cfg.Width, cfg.Height, perPixel) {
		return nil, ErrTooBig
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// tooBig reports whether decoding a w x h picture at perPixel bytes a pixel
// takes more than maxDecodeBytes. It is worked out in floating point so that
// a huge size can't overflow.
func tooBig(w, h int, perPixel float64) bool {
	return w <= 0 || h <= 0 || float64(w)*float64(h)*perPixel > maxDecodeBytes
}

// jpegCost estimates the bytes a pixel needed to decode the JPEG in data, from
// its frame header: one for each decoded plane, scaled by the share of the
// picture's area the plane covers, and four more for each when the JPEG is
// progressive. ok is false when no frame header is found.
//
// The markers are walked as image/jpeg walks them, so that a crafted file
// can't show this one frame header and the decoder another: TEM and a second
// SOI have a length like any other segment.
func jpegCost(data []byte) (perPixel float64, ok bool) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, false
	}
	for i := 2; i+2 <= len(data); {
		if data[i] != 0xFF { // not a marker: skipped a byte at a time, as image/jpeg does
			i++
			continue
		}
		m := data[i+1]
		switch {
		case m == 0xFF: // fill
			i++
			continue
		case m == 0x00 || m >= 0xD0 && m <= 0xD7: // FF 00 and the restart markers have no length
			i += 2
			continue
		case m == 0xD9 || m == 0xDA: // the end, or the scan: no frame header came first
			return 0, false
		}
		if i+4 > len(data) {
			return 0, false
		}
		end := i + 2 + int(data[i+2])<<8 + int(data[i+3])
		if end < i+4 || end > len(data) {
			return 0, false
		}
		if isSOF(m) {
			return frameCost(data[i+4:end], m == 0xC2 || m == 0xC6 || m == 0xCA || m == 0xCE)
		}
		i = end
	}
	return 0, false
}

// pngCost is the bytes a pixel of the PNG in data decodes to: 8 for 16 bits a
// channel, 4 otherwise. The bit depth is the byte after the IHDR's width and
// height.
func pngCost(data []byte) float64 {
	if len(data) > 24 && data[24] == 16 {
		return 8
	}
	return 4
}

// isSOF reports whether marker m starts a frame header.
func isSOF(m byte) bool {
	return m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC
}

// frameCost is jpegCost for the body of a frame header: the precision, size
// and component count, then each component's id, sampling factors and table.
func frameCost(seg []byte, progressive bool) (float64, bool) {
	if len(seg) < 6 {
		return 0, false
	}
	n := int(seg[5])
	if n == 0 || len(seg) < 6+3*n {
		return 0, false
	}
	hmax, vmax := 0, 0
	for c := range n {
		h, v := int(seg[7+3*c]>>4), int(seg[7+3*c]&15)
		if h == 0 || v == 0 {
			return 0, false
		}
		hmax, vmax = max(hmax, h), max(vmax, v)
	}
	var share float64
	for c := range n {
		h, v := int(seg[7+3*c]>>4), int(seg[7+3*c]&15)
		share += float64(h*v) / float64(hmax*vmax)
	}
	if progressive {
		share *= 5
	}
	return share, true
}

// Square crops src to its centred square and scales it down to px by px,
// each output pixel the average of the source pixels it covers. Album art is
// square almost always; a wide or tall picture loses its edges, as covers do
// elsewhere. Nothing is scaled up: px larger than the square is reduced to
// it. The result is opaque.
func Square(src image.Image, px int) *image.NRGBA {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	px = min(px, side)
	if px <= 0 {
		return image.NewNRGBA(image.Rectangle{})
	}
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	spans := coverage(side, px)
	acc := make([]float32, 3*px*px) // the output, summed in rows as source rows arrive
	row := make([]float32, 3*side)
	scaled := make([]float32, 3*px)
	for y := range side {
		readRow(src, x0, y0+y, side, row)
		clear(scaled)
		for x := range side {
			r, g, bl := row[3*x], row[3*x+1], row[3*x+2]
			for _, s := range spans[x] {
				o := 3 * s.out
				scaled[o] += r * s.w
				scaled[o+1] += g * s.w
				scaled[o+2] += bl * s.w
			}
		}
		for _, s := range spans[y] {
			line := acc[3*px*s.out : 3*px*(s.out+1)]
			for i, v := range scaled {
				line[i] += v * s.w
			}
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, px, px))
	for i := range px * px {
		out.Pix[4*i], out.Pix[4*i+1], out.Pix[4*i+2], out.Pix[4*i+3] = to8(acc[3*i]), to8(acc[3*i+1]), to8(acc[3*i+2]), 255
	}
	return out
}

// span is a source pixel's share of one output pixel.
type span struct {
	out int
	w   float32
}

// coverage lists, for each of n source pixels, the output pixels among m that
// it falls in and with what weight; each output pixel's weights add up to 1.
func coverage(n, m int) [][]span {
	spans := make([][]span, n)
	scale := float64(n) / float64(m)
	for o := range m {
		lo, hi := float64(o)*scale, float64(o+1)*scale
		for i := int(lo); i < n && float64(i) < hi; i++ {
			if w := (math.Min(hi, float64(i+1)) - math.Max(lo, float64(i))) / scale; w > 0 {
				spans[i] = append(spans[i], span{o, float32(w)})
			}
		}
	}
	return spans
}

// readRow reads n pixels of row y from x0 into row as red, green and blue
// from 0 to 255. Transparent pixels count as black, as they would over the
// dark ground.
func readRow(src image.Image, x0, y, n int, row []float32) {
	switch s := src.(type) {
	case *image.YCbCr:
		for i := range n {
			yi, ci := s.YOffset(x0+i, y), s.COffset(x0+i, y)
			r, g, b := color.YCbCrToRGB(s.Y[yi], s.Cb[ci], s.Cr[ci])
			row[3*i], row[3*i+1], row[3*i+2] = float32(r), float32(g), float32(b)
		}
	case *image.NRGBA:
		p := s.Pix[s.PixOffset(x0, y):]
		for i := range n {
			a := float32(p[4*i+3]) / 255
			row[3*i], row[3*i+1], row[3*i+2] = float32(p[4*i])*a, float32(p[4*i+1])*a, float32(p[4*i+2])*a
		}
	case *image.RGBA:
		p := s.Pix[s.PixOffset(x0, y):]
		for i := range n {
			row[3*i], row[3*i+1], row[3*i+2] = float32(p[4*i]), float32(p[4*i+1]), float32(p[4*i+2])
		}
	case *image.Gray:
		p := s.Pix[s.PixOffset(x0, y):]
		for i := range n {
			v := float32(p[i])
			row[3*i], row[3*i+1], row[3*i+2] = v, v, v
		}
	default:
		for i := range n {
			r, g, b, _ := src.At(x0+i, y).RGBA() // premultiplied, so over black
			row[3*i], row[3*i+1], row[3*i+2] = float32(r>>8), float32(g>>8), float32(b>>8)
		}
	}
}

func to8(v float32) uint8 { return uint8(min(max(v+0.5, 0), 255)) }

// Colour is the art's dominant hue and chroma (docs/ui.md, "Album accent"):
// the art is averaged down to 48 x 48 and every pixel of that sampled. ok is
// false for greyscale art.
func Colour(img *image.NRGBA) (hue, chroma float64, ok bool) {
	return theme.Dominant(Square(img, 48))
}

// backdropPx is the Lyrics backdrop's size. The screen scales it up with
// linear filtering, which a picture this blurred survives.
const backdropPx = 64

// Backdrop is the Lyrics screen's background made from the art (docs/ui.md,
// "Backgrounds"): 64 x 64, blurred by about 5 % of its width, as the design's
// 70 px blur is of its 1400 px art, and 20 % more saturated.
func Backdrop(img *image.NRGBA) *image.NRGBA {
	b := Square(img, backdropPx)
	raster.Blur(b, 3)
	saturate(b, 1.2)
	return b
}

// saturate scales the colourfulness of img by s with the matrix CSS's
// saturate() filter uses.
func saturate(img *image.NRGBA, s float32) {
	for i := 0; i+3 < len(img.Pix); i += 4 {
		r, g, b := float32(img.Pix[i]), float32(img.Pix[i+1]), float32(img.Pix[i+2])
		img.Pix[i] = to8((0.213+0.787*s)*r + (0.715-0.715*s)*g + (0.072-0.072*s)*b)
		img.Pix[i+1] = to8((0.213-0.213*s)*r + (0.715+0.285*s)*g + (0.072-0.072*s)*b)
		img.Pix[i+2] = to8((0.213-0.213*s)*r + (0.715-0.715*s)*g + (0.072+0.928*s)*b)
	}
}
