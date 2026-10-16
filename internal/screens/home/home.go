// Package home is the Home tab: rows of recent and saved music (docs/ui.md,
// screen 01).
package home

import (
	"time"

	"github.com/ujjwal313/boombrick/internal/fixture"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/focus"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
	"github.com/ujjwal313/boombrick/internal/ui/text"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// Layout from docs/ui.md, screen 01, in device pixels.
const (
	contentTop = widgets.StatusHeight + widgets.TabsHeight
	contentBot = theme.ScreenH - widgets.HintsHeight
	padTop     = 14
	side       = 32
	cardSize   = 200
	tileSize   = 166
	cardGap    = 32
	tileGap    = 28
	headingGap = 20 // heading to its items
	rowGap     = 30 // items to the next heading: a 20 px gap plus a 10 px margin
	captionGap = 12 // card art to caption
	radius     = 12
)

// Home shows rows of cards and tiles and moves focus between them.
type Home struct {
	rows     []fixture.Row
	row, col int
	scroll   anim.Value
	offsets  []anim.Value           // each row's sideways scroll
	focus    map[[2]int]*anim.Value // per-item focus strength, 0 to 1
	tops     []float64              // each row's heading, from the top of the content
}

// New creates the screen with focus on the first item.
func New(rows []fixture.Row) *Home {
	h := &Home{rows: rows, offsets: make([]anim.Value, len(rows)), focus: map[[2]int]*anim.Value{}}
	h.value(0, 0).Set(1)
	return h
}

func (h *Home) value(row, col int) *anim.Value {
	k := [2]int{row, col}
	v, ok := h.focus[k]
	if !ok {
		v = &anim.Value{}
		h.focus[k] = v
	}
	return v
}

// layout measures the rows once text metrics are available.
func (h *Home) layout(u *ui.Context) {
	if h.tops != nil {
		return
	}
	headH := float64(u.LineHeight(text.Heading))
	cardH := float64(cardSize+captionGap+2) + float64(u.LineHeight(text.Body)+u.LineHeight(text.Meta))
	h.tops = make([]float64, len(h.rows))
	y := 0.0
	for i, r := range h.rows {
		h.tops[i] = y
		itemH := float64(tileSize)
		if r.Cards {
			itemH = cardH
		}
		y += headH + headingGap + itemH + rowGap
	}
}

// scrollTarget puts the focused row's heading directly under the tabs. The
// last row snaps there too, even though that leaves space below it, so its
// focused item is never clipped by the hint bar.
func (h *Home) scrollTarget() float64 {
	return h.tops[h.row]
}

// itemSize is a row's item size and the gap between its items.
func itemSize(r fixture.Row) (size, gap float64) {
	if r.Cards {
		return cardSize, cardGap
	}
	return tileSize, tileGap
}

// offsetTarget scrolls a row sideways as little as possible to keep item col
// between the side paddings. Its focus zoom and ring spill into them, as on
// the first item.
func (h *Home) offsetTarget(row, col int) float64 {
	size, gap := itemSize(h.rows[row])
	left := float64(col) * (size + gap)
	return focus.Scroll(h.offsets[row].Target(), left, left+size, theme.ScreenW-2*side)
}

// Handle moves focus with the D-pad.
func (h *Home) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind == input.Release {
		return false
	}
	row, col := h.row, h.col
	switch ev.Button {
	case input.Left:
		col--
	case input.Right:
		col++
	case input.Up:
		row--
	case input.Down:
		row++
	default:
		return false
	}
	row = min(max(row, 0), len(h.rows)-1)
	col = min(max(col, 0), len(h.rows[row].Items)-1)
	if row == h.row && col == h.col {
		return false
	}
	h.value(h.row, h.col).To(0, now, anim.Focus, anim.EaseOutCubic)
	h.row, h.col = row, col
	h.value(row, col).To(1, now, anim.Focus, anim.EaseOutCubic)
	h.offsets[row].To(h.offsetTarget(row, col), now, anim.Focus, anim.EaseOutCubic)
	if h.tops != nil {
		h.scroll.To(h.scrollTarget(), now, anim.Focus, anim.EaseOutCubic)
	}
	return true
}

// Animating reports whether focus or scrolling is still moving.
func (h *Home) Animating(now time.Time) bool {
	if !h.scroll.Done(now) {
		return true
	}
	for i := range h.offsets {
		if !h.offsets[i].Done(now) {
			return true
		}
	}
	for _, v := range h.focus {
		if !v.Done(now) {
			return true
		}
	}
	return false
}

// Draw renders the frame.
func (h *Home) Draw(u *ui.Context) {
	h.layout(u)
	now := u.Now
	widgets.DrawBackground(u, widgets.TabGround)

	u.PushClip(gfx.Rect{X: 0, Y: contentTop, W: theme.ScreenW, H: contentBot - contentTop})
	base := float32(contentTop+padTop) - float32(h.scroll.At(now))
	var focused func()
	for ri, r := range h.rows {
		top := base + float32(h.tops[ri])
		drawHeading(u, r, top)
		itemsTop := top + u.LineHeight(text.Heading) + headingGap
		s, g := itemSize(r)
		size, gap := float32(s), float32(g)
		shift := float32(h.offsets[ri].At(now))
		for ci, it := range r.Items {
			x := side + float32(ci)*(size+gap) - shift
			k := float32(h.value(ri, ci).At(now))
			draw := func() { drawItem(u, r, it, x, itemsTop, size, k) }
			if ri == h.row && ci == h.col {
				focused = draw // drawn last so its ring and glow sit on top
				continue
			}
			draw()
		}
	}
	if focused != nil {
		focused()
	}
	u.PopClip()

	widgets.StatusBar(u)
	widgets.Tabs(u, 0)
	widgets.HintBar(u, h.hints())
}

func drawHeading(u *ui.Context, r fixture.Row, top float32) {
	u.Text(r.Title, text.Heading, side, top, theme.Paper)
	baseline := top + u.Ascent(text.Heading)
	w := u.TextWidth(r.Meta, text.Meta)
	u.Text(r.Meta, text.Meta, theme.ScreenW-side-w, u.Baseline(text.Meta, baseline), theme.Ink3)
}

// drawItem draws a card or tile. As focus k goes from 0 to 1 it scales to
// 106 % from its top centre and gains the focus ring.
func drawItem(u *ui.Context, r fixture.Row, it fixture.Item, x, y, size, k float32) {
	u.PushScale(x+size/2, y, 1+0.06*k)
	defer u.Pop()

	box := gfx.Rect{X: x, Y: y, W: size, H: size}
	widgets.FocusRing(u, box, radius, k, theme.Ground)
	widgets.Art(u, it.Art, int(size), radius, box)
	if r.Continue {
		widgets.Badge(u, it.Source, x+10, y+10, false)
		progressStrip(u, box, it.Progress)
	}
	if !r.Cards {
		return
	}
	ty := y + size + captionGap
	u.Text(u.Fit(it.Title, text.Body, size), text.Body, x, ty, theme.Paper)
	u.Text(u.Fit(it.Subtitle, text.Meta, size), text.Meta, x, ty+u.LineHeight(text.Body)+2, theme.Ink3)
}

// progressStrip draws the 6 px progress bar along the bottom of the art,
// clipped to its rounded corners.
func progressStrip(u *ui.Context, box gfx.Rect, progress float64) {
	mask := raster.Shape{W: int(box.W), H: int(box.H), Radius: radius, Kind: raster.Fill}
	strip := gfx.Rect{X: box.X, Y: box.Y + box.H - 6, W: box.W, H: 6}
	u.PushClip(strip)
	u.Shape(mask, box, theme.ArtShade)
	u.PopClip()
	strip.W = box.W * float32(progress)
	u.PushClip(strip)
	u.Shape(mask, box, u.Accent.Color)
	u.PopClip()
}

func (h *Home) hints() []widgets.Hint {
	if h.rows[h.row].Spotify {
		return []widgets.Hint{
			{Key: "A", Label: "Open"}, {Key: "X", Label: "Import"},
			{Key: "Y", Label: "Add to queue"}, {Key: "L1/R1", Label: "Tabs"},
		}
	}
	return []widgets.Hint{
		{Key: "A", Label: "Play"}, {Key: "B", Label: "Back"}, {Key: "X", Label: "Download"},
		{Key: "Y", Label: "Add to queue"}, {Key: "L1/R1", Label: "Tabs"},
	}
}
