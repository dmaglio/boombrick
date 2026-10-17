// Package home is the Home tab: rows of recent and saved music (docs/ui.md,
// screen 01).
package home

import (
	"fmt"
	"slices"
	"time"

	"github.com/ujjwal313/boombrick/internal/fixture"
	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/media"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/resume"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/focus"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/text"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// Layout from docs/ui.md, screen 01, in design pixels.
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
)

// Home shows rows of cards and tiles and moves focus between them.
type Home struct {
	rows     []fixture.Row
	row, col int
	scroll   anim.Value
	offsets  []anim.Value           // each row's sideways scroll
	focus    map[[2]int]*anim.Value // per-item focus strength, 0 to 1
	tops     []float64              // each row's heading, from the top of the content
	set      bool                   // rows have been given, even if there are none
	reading  bool                   // the first scan is running with nothing indexed yet
	musicDir string                 // shown on the empty state
	rescan   func()
	state    widgets.Wrapped // the empty or reading state's description, wrapped
}

// New creates the screen with focus on the first item. With no rows it shows
// only the chrome until SetRows.
func New(rows []fixture.Row) *Home {
	h := &Home{rows: rows, set: len(rows) > 0, offsets: make([]anim.Value, len(rows)), focus: map[[2]int]*anim.Value{}}
	h.value(0, 0).Set(1)
	return h
}

// SetEmpty sets what the empty state shows and what Y does there.
func (h *Home) SetEmpty(musicDir string, rescan func()) { h.musicDir, h.rescan = musicDir, rescan }

// SetReading says the first scan is running and nothing is indexed yet;
// until SetRows the screen says the music is being read.
func (h *Home) SetReading() { h.reading = true }

// SetRows replaces the rows, keeping focus on the same row and item where
// they still exist (found by title, and by title and subtitle). Otherwise
// focus stays at its position, clamped. Rows with no items are dropped.
func (h *Home) SetRows(rows []fixture.Row) {
	var rowTitle, title, subtitle string
	offsets := map[string]anim.Value{}
	for i, r := range h.rows {
		if i < len(h.offsets) {
			offsets[r.Title] = h.offsets[i]
		}
	}
	if h.row < len(h.rows) {
		r := h.rows[h.row]
		rowTitle = r.Title
		if h.col < len(r.Items) {
			title, subtitle = r.Items[h.col].Title, r.Items[h.col].Subtitle
		}
	}
	h.rows = slices.DeleteFunc(slices.Clone(rows), func(r fixture.Row) bool { return len(r.Items) == 0 })
	h.set = true
	h.tops = nil
	clear(h.focus)
	if len(h.rows) == 0 {
		h.row, h.col = 0, 0
		h.offsets = nil
		h.scroll.Set(0)
		return
	}
	h.offsets = make([]anim.Value, len(h.rows))
	for i, r := range h.rows {
		h.offsets[i] = offsets[r.Title]
	}
	if i := slices.IndexFunc(h.rows, func(r fixture.Row) bool { return r.Title == rowTitle }); i >= 0 {
		h.row = i
		if j := slices.IndexFunc(h.rows[i].Items, func(it fixture.Item) bool {
			return it.Title == title && it.Subtitle == subtitle
		}); j >= 0 {
			h.col = j
		}
	}
	h.row = min(h.row, len(h.rows)-1)
	h.col = min(h.col, len(h.rows[h.row].Items)-1)
	h.value(h.row, h.col).Set(1)
	// Rows that lost items may be scrolled past their end, and the focused
	// item may have moved; bring it into view.
	for i := range h.offsets {
		if m := h.maxOffset(i); h.offsets[i].Target() > m {
			h.offsets[i].Set(m)
		}
	}
	h.offsets[h.row].Set(h.offsetTarget(h.row, h.col))
}

// maxRecent is how many albums the Recently added row shows.
const maxRecent = 12

// maxContinue is how many albums Continue listening shows (docs/ui.md,
// screen 01: up to four cards).
const maxContinue = 4

// ContinueRow is Continue listening: albums part-way through, most recent
// first, skipping any no longer in the library. play resumes an album at a
// track and frame; queue adds it to the queue.
func ContinueRow(progress []resume.Progress, ix *library.Index, play func(a *library.Album, track int, frame int64),
	queue func(*library.Album)) (fixture.Row, bool) {
	var items []fixture.Item
	for _, pr := range progress {
		a := ix.Album(pr.Artist, pr.Album)
		if a == nil {
			continue
		}
		track := slices.IndexFunc(a.Tracks, func(t library.Track) bool { return t.Path == pr.Path })
		if track < 0 {
			continue
		}
		frame := pr.Frame
		// The badge is the continued track's: an album may mix downloads with files of its own.
		it := fixture.Item{Title: a.Title, Subtitle: a.Artist, Art: screens.AlbumArt(a),
			Source: screens.SourceOf(a.Tracks[track].Path), Progress: pr.Fraction, Play: func() { play(a, track, frame) }}
		if queue != nil {
			it.Queue = func() { queue(a) }
		}
		items = append(items, it)
		if len(items) == maxContinue {
			break
		}
	}
	if len(items) == 0 {
		return fixture.Row{}, false
	}
	return fixture.Row{Title: "Continue listening", Meta: fmt.Sprintf("%d in progress", len(items)), Cards: true,
		Continue: true, Items: items}, true
}

// AlbumRow is the Recently added row for albums found on the SD card, newest
// first; play starts an album and queue adds it to the queue. It reports false
// when there are no albums.
func AlbumRow(albums []*library.Album, play, queue func(*library.Album)) (fixture.Row, bool) {
	if len(albums) == 0 {
		return fixture.Row{}, false
	}
	items := make([]fixture.Item, min(len(albums), maxRecent))
	for i := range items {
		a := albums[i]
		items[i] = fixture.Item{Title: a.Title, Subtitle: a.Artist, Art: screens.AlbumArt(a), Source: source(a),
			Play: func() { play(a) }}
		if queue != nil {
			items[i].Queue = func() { queue(a) }
		}
	}
	meta := fmt.Sprintf("%d albums", len(albums))
	if len(albums) == 1 {
		meta = "1 album"
	}
	return fixture.Row{Title: "Recently added", Meta: meta, Items: items}, true
}

// source is where album a's files came from, for its badge: Downloaded for
// an album filed under the downloads folder.
func source(a *library.Album) media.Source {
	if len(a.Tracks) == 0 {
		return media.Local
	}
	return screens.SourceOf(a.Tracks[0].Path)
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
	if h.tops != nil || len(h.rows) == 0 {
		return
	}
	headH := float64(u.LineHeight(text.Heading))
	cardH := float64(widgets.CardHeight(u, cardSize))
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
	h.scroll.Set(h.scrollTarget())
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

// maxOffset is the furthest a row scrolls sideways: its last item at the
// right-hand padding, or 0 when the row fits.
func (h *Home) maxOffset(row int) float64 {
	size, gap := itemSize(h.rows[row])
	n := float64(len(h.rows[row].Items))
	return max(0, n*size+(n-1)*gap-(theme.ScreenW-2*side))
}

// Handle moves focus with the D-pad.
func (h *Home) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind == input.Release {
		return false
	}
	if len(h.rows) == 0 {
		if ev.Button == input.Y && ev.Kind == input.Press && h.set && h.rescan != nil {
			h.rescan()
			return true
		}
		return false
	}
	if ev.Button == input.A || ev.Button == input.Y {
		it := h.rows[h.row].Items[h.col]
		act := it.Play
		if ev.Button == input.Y {
			act = it.Queue
		}
		if act != nil && ev.Kind == input.Press {
			act()
			return true
		}
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
	if len(h.rows) == 0 {
		h.drawEmpty(u)
		return
	}
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
			draw := func() {
				widgets.CardGlow(u, x, itemsTop, size, k)
				widgets.Card(u, widgets.CardItem{Title: it.Title, Subtitle: it.Subtitle, Art: it.Art, Source: it.Source,
					Badge: r.Continue, Progress: it.Progress}, x, itemsTop, size, k, r.Cards)
			}
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

// drawEmpty draws the chrome alone before the first rows arrive, or that the
// first scan is reading the music, then the empty-library state.
func (h *Home) drawEmpty(u *ui.Context) {
	switch {
	case h.set:
		widgets.DrawBackground(u, widgets.StateGround)
		widgets.EmptyState(u, &h.state, "Your library is empty",
			"Copy music to the Music folder on the SD card, or find something on YouTube Music.", h.musicDir)
	case h.reading:
		screens.DrawReading(u, &h.state, h.musicDir)
	default:
		widgets.DrawBackground(u, widgets.TabGround)
	}
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

func (h *Home) hints() []widgets.Hint {
	if len(h.rows) == 0 {
		if h.set {
			return []widgets.Hint{{Key: "Y", Label: "Rescan SD card"}, {Key: "L1/R1", Label: "Tabs"}}
		}
		return []widgets.Hint{{Key: "L1/R1", Label: "Tabs"}}
	}
	if it := h.rows[h.row].Items[h.col]; it.Play != nil {
		hints := []widgets.Hint{{Key: "A", Label: "Play"}}
		if it.Queue != nil {
			hints = append(hints, widgets.Hint{Key: "Y", Label: "Add to queue"})
		}
		return append(hints, widgets.Hint{Key: "L1/R1", Label: "Tabs"})
	}
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
