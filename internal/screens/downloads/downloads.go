// Package downloads is the Downloads tab: the SD card's space, and the
// download queue in sections, with a dialog for failures (docs/ui.md,
// screens 09 and 12c).
package downloads

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"github.com/ujjwal313/boombrick/internal/download"
	"github.com/ujjwal313/boombrick/internal/media"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
	"github.com/ujjwal313/boombrick/internal/ui/text"
	"github.com/ujjwal313/boombrick/internal/ui/views"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
	"github.com/ujjwal313/boombrick/internal/ytmusic"
)

// Source is the download queue as the tab uses it. *download.Service is one.
type Source interface {
	Status() download.Status
	Retry(videoID string, update bool)
	Remove(videoID string)
	SetPaused(on bool)
}

// tab is the Downloads tab's place in the tab row.
const tab = 4

// Layout (docs/ui.md, 09): the storage card under the tabs, then the queue
// in 66 px rows, each heading a row of its own.
const (
	cardTop = screens.ContentTop + 8
	cardH   = 112
	listTop = cardTop + cardH
	rowH    = 66
	artSize = 50
	barH    = 5
	retryH  = 40 // the Retry pill on a failed row
)

var (
	cardTitle  = text.Style{Face: text.Lora, Size: 26}
	headText   = text.Style{Face: text.Lora, Size: 20}
	titleText  = text.Style{Face: text.Lora, Size: 24}
	statusText = text.Style{Face: text.Lora, Size: 20}
)

// line is a row of the list: a section's heading, the downloader being
// fetched, or a download. Its strings are made when the queue changes, not
// every frame.
type line struct {
	head, note    string // a heading, and what it says on the right
	install       bool
	entry         download.Entry
	art           widgets.ArtSpec
	title, status string
	progress      float32 // the bar under the title, or -1 for none
}

// Downloads is the Downloads tab.
type Downloads struct {
	nav     screens.Nav
	src     Source
	version uint64
	status  download.Status
	lines   []line
	list    views.Grid
	summary string    // the card's "used · free of" line
	legend  [3]string // music, other, free
	dlg     *failure
	retryW  float32 // the Retry pill's width, measured on the first draw
	hintFor hintKey
	hintBar []widgets.Hint
}

// hintKey is what the tab's hints depend on.
type hintKey struct {
	focus  int
	state  media.DownloadState
	on     bool // focus is on a download
	paused bool
}

// New returns the tab.
func New(nav screens.Nav, src Source) *Downloads {
	return &Downloads{nav: nav, src: src, list: views.Grid{Cols: 1, CellW: theme.ScreenW - 2*screens.Side,
		CellH: rowH, Pad: screens.Pad, ViewH: screens.ContentBot - listTop}}
}

// refresh takes the queue's status, and makes the list again when it has
// changed, keeping focus on the same download. When that download is gone,
// focus stays at its place in the list, or the nearest download to it.
func (d *Downloads) refresh() {
	st := d.src.Status()
	if st.Version == d.version {
		return
	}
	focused := ""
	if e, ok := d.focused(); ok {
		focused = e.Song.VideoID
	}
	old := d.list.Focus()
	d.version, d.status = st.Version, st
	d.lines = sections(st)
	d.summary, d.legend = storage(st.Storage)
	f := -1
	for i := range d.lines {
		if focused != "" && !d.lines[i].skip() && d.lines[i].entry.Song.VideoID == focused {
			f = i
			break
		}
	}
	if f < 0 {
		f = d.nearest(min(old, len(d.lines)-1))
	}
	d.list.SetCountFocus(len(d.lines), max(f, 0))
	if f >= 0 && d.list.Focus() != f { // skip the heading a fresh list starts on
		d.list.SetFocus(f, time.Time{})
	}
}

// nearest is the first line at or after i that focus can rest on, or else the
// last one before it; -1 if there is none.
func (d *Downloads) nearest(i int) int {
	for j := max(i, 0); j < len(d.lines); j++ {
		if !d.lines[j].skip() {
			return j
		}
	}
	for j := min(i, len(d.lines)) - 1; j >= 0; j-- {
		if !d.lines[j].skip() {
			return j
		}
	}
	return -1
}

// skip reports whether focus passes over the line: a heading, or the
// downloader being fetched.
func (l *line) skip() bool { return l.head != "" || l.install }

// sections groups the queue under its headings: under way and queued,
// failed, and saved today.
func sections(st download.Status) []line {
	var active, failed, done []line
	if st.Installing {
		active = append(active, line{install: true, title: "Getting the downloader",
			status: strconv.Itoa(int(st.Install*100+0.5)) + " %", progress: st.Install})
	}
	for _, e := range st.Entries {
		l := line{entry: e, art: art(e.Song), title: e.Song.Title + " — " + e.Song.Artist, progress: -1}
		switch e.State {
		case media.Downloading:
			l.status, l.progress = strconv.Itoa(int(e.Progress*100+0.5))+" %", e.Progress
			if e.Speed > 0 {
				l.status += " · " + size(int64(e.Speed)) + "/s"
			}
			active = append(active, l)
		case media.Queued:
			l.status = "Queued"
			active = append(active, l)
		case media.Failed:
			l.status = failureText[e.Failure]
			failed = append(failed, l)
		case media.Saved:
			l.status = size(e.Bytes)
			done = append(done, l)
		}
	}
	note := queueNote(st)
	var lines []line
	for _, s := range []struct {
		name  string
		lines []line
	}{{"ACTIVE", active}, {"FAILED", failed}, {"COMPLETED TODAY", done}} {
		if len(s.lines) > 0 {
			// The queue's state sits on the first heading, which is the Active
			// one unless nothing is active.
			lines = append(lines, line{head: s.name + " · " + strconv.Itoa(len(s.lines)), note: note})
			note = ""
			lines = append(lines, s.lines...)
		}
	}
	return lines
}

// queueNote is what the queue says about itself: paused, or why it waits.
func queueNote(st download.Status) string {
	if st.Paused {
		return "Paused"
	}
	return st.Waiting
}

// failureText is what a failed row says.
var failureText = map[download.Failure]string{
	download.NeedsUpdate: "Couldn’t fetch audio",
	download.Network:     "Connection dropped",
	download.Unavailable: "Not available",
	download.Other:       "Couldn’t save it",
}

func art(s ytmusic.Song) widgets.ArtSpec {
	a := widgets.SpecFor(s.Album)
	if s.Thumb != "" {
		a.Cover = media.Cover{URL: s.Thumb}
	}
	return a
}

// storage makes the card's lines.
func storage(s download.Storage) (string, [3]string) {
	used := s.Total - s.Free
	return size(used) + " used · " + size(s.Free) + " free of " + size(s.Total),
		[3]string{"Music " + size(s.Music), "Other " + size(max(used-s.Music, 0)), "Free " + size(s.Free)}
}

// size is a byte count as the tab writes it: "9.4 MB", "59.5 GB".
func size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return strconv.FormatInt((n+1023)/1024, 10) + " KB"
}

// focused is the download focus is on, if focus is on one.
func (d *Downloads) focused() (download.Entry, bool) {
	f := d.list.Focus()
	if f < 0 || f >= len(d.lines) || d.lines[f].skip() {
		return download.Entry{}, false
	}
	return d.lines[f].entry, true
}

// Handle acts on the queue as the last frame drew it. It opens the failure
// dialog with A, removes with X, pauses with Y and moves with the D-pad; the
// dialog takes input while it is open.
func (d *Downloads) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind == input.Release {
		return false
	}
	if d.dlg != nil {
		return d.dlg.handle(d, ev, now)
	}
	if ev.Kind == input.Press {
		e, ok := d.focused()
		switch ev.Button {
		case input.A:
			if !ok || e.State != media.Failed {
				return false
			}
			d.dlg = newFailure(e, now)
			return true
		case input.X:
			if !ok {
				return false
			}
			d.remove(e)
			return true
		case input.Y:
			d.src.SetPaused(!d.status.Paused)
			return true
		}
	}
	return d.move(ev.Button, now)
}

// remove removes a download and says so.
func (d *Downloads) remove(e download.Entry) {
	d.src.Remove(e.Song.VideoID)
	d.nav.Toast("Removed “" + e.Song.Title + "”")
}

// move moves focus up or down, passing over headings.
func (d *Downloads) move(b input.Button, now time.Time) bool {
	step := 0
	switch b {
	case input.Up:
		step = -1
	case input.Down:
		step = 1
	default:
		return false
	}
	for i := d.list.Focus() + step; i >= 0 && i < len(d.lines); i += step {
		if !d.lines[i].skip() {
			d.list.SetFocus(i, now)
			return true
		}
	}
	return false
}

// Claims takes Start and Select while the dialog is open, so nothing opens
// over it.
func (d *Downloads) Claims(b input.Button) bool {
	return d.dlg != nil && (b == input.Start || b == input.Select)
}

// Animating reports whether focus, the list or the dialog is moving.
func (d *Downloads) Animating(now time.Time) bool {
	return d.list.Animating(now) || d.dlg != nil && d.dlg.animating(now)
}

// ShowsDownloads is true: the tab is the queue.
func (d *Downloads) ShowsDownloads() bool { return true }

// Draw renders the frame.
func (d *Downloads) Draw(u *ui.Context) {
	d.refresh()
	widgets.DrawBackground(u, widgets.TabGround)
	d.drawCard(u)
	if len(d.lines) == 0 {
		d.drawEmpty(u)
	} else {
		d.drawList(u)
	}
	widgets.StatusBar(u)
	widgets.Tabs(u, tab)
	if d.dlg != nil {
		if d.dlg.gone(u.Now) {
			d.dlg = nil
		} else {
			d.dlg.draw(u)
			widgets.HintBar(u, d.dlg.hints())
			return
		}
	}
	widgets.HintBar(u, d.hints())
}

func (d *Downloads) drawCard(u *ui.Context) {
	box := gfx.Rect{X: screens.Side, Y: cardTop, W: theme.ScreenW - 2*screens.Side, H: cardH}
	u.Shape(raster.Shape{W: int(box.W), H: cardH, Radius: 14, Kind: raster.Ring, RingOffset: -1, RingWidth: 1}, box,
		theme.PaperA(0.14))
	x, y := box.X+20, box.Y+14
	u.Icon("sd", 26, 2, false, x, y+4, theme.Paper)
	u.Text("SD card", cardTitle, x+36, u.CenterY(cardTitle, y, 34), theme.Paper)
	st := d.status.Storage
	if st.Total <= 0 {
		return
	}
	u.Text(d.summary, text.Meta, box.X+box.W-20-u.TextWidth(d.summary, text.Meta), u.CenterY(text.Meta, y, 34), theme.Ink2)
	bar := gfx.Rect{X: x, Y: y + 34 + 8, W: box.W - 40, H: 10}
	shape := raster.Shape{W: int(bar.W), H: 10, Radius: 5, Kind: raster.Fill}
	u.Shape(shape, bar, theme.PaperA(0.12))
	music := bar.W * float32(st.Music) / float32(st.Total)
	other := bar.W * float32(max(st.Total-st.Free-st.Music, 0)) / float32(st.Total)
	u.PushClip(gfx.Rect{X: bar.X, Y: bar.Y, W: music, H: 10})
	u.Shape(shape, bar, u.Accent.Color)
	u.PopClip()
	u.PushClip(gfx.Rect{X: bar.X + music, Y: bar.Y, W: other, H: 10})
	u.Shape(shape, bar, theme.Ink3)
	u.PopClip()
	ly := bar.Y + 10 + 8
	lx := x
	for i, col := range [3]color.NRGBA{u.Accent.Color, theme.Ink3, theme.PaperA(0.12)} {
		u.Shape(raster.Shape{W: 12, H: 12, Radius: 3, Kind: raster.Fill}, gfx.Rect{X: lx, Y: ly + 7, W: 12, H: 12}, col)
		lx += 20 + u.Text(d.legend[i], statusText, lx+20, ly, theme.Ink2) + 24
	}
}

func (d *Downloads) drawEmpty(u *ui.Context) {
	top := float32(listTop + 40)
	title, body := "Nothing downloading", "Songs you download from YouTube Music show up here."
	if note := queueNote(d.status); note != "" {
		body = note
	}
	u.Text(title, text.Heading, (theme.ScreenW-u.TextWidth(title, text.Heading))/2, top, theme.Paper)
	u.Text(body, text.Meta, (theme.ScreenW-u.TextWidth(body, text.Meta))/2, top+u.LineHeight(text.Heading)+8, theme.Ink3)
}

func (d *Downloads) drawList(u *ui.Context) {
	u.PushClip(screens.GlowArea)
	d.list.Draw(u.Now, screens.Side, listTop, func(i int, x, y, k float32) {
		l := &d.lines[i]
		box := gfx.Rect{X: x, Y: y, W: d.list.CellW, H: rowH}
		if !l.skip() {
			widgets.RowGlow(u, box, widgets.RowRadius, k)
		}
		u.PushClip(screens.Below(listTop))
		defer u.PopClip()
		if l.head != "" {
			ty := y + rowH - 6 - u.LineHeight(headText)
			u.Text(l.head, headText, x+16, ty, theme.Ink3)
			if l.note != "" {
				u.Text(l.note, headText, x+box.W-16-u.TextWidth(l.note, headText), ty, theme.GoldSoft)
			}
			return
		}
		if !l.install {
			widgets.RowFocus(u, box, widgets.RowRadius, k)
		}
		tx := x + 16
		if !l.install {
			widgets.Art(u, l.art, artSize, 8, gfx.Rect{X: tx, Y: y + (rowH-artSize)/2, W: artSize, H: artSize})
			tx += artSize + 16
		}
		right := x + box.W - 16
		if l.entry.State == media.Failed {
			if d.retryW == 0 {
				d.retryW = float32(14 + 20 + 8 + u.TextWidth("Retry", statusText) + 14)
			}
			retryPill(u, d.retryW, right, y+(rowH-retryH)/2)
			right -= d.retryW + 14
		}
		col := theme.Ink2
		if l.entry.State == media.Failed {
			col = theme.GoldSoft
		}
		sw := u.TextWidth(l.status, statusText)
		u.Text(l.status, statusText, right-sw, u.CenterY(statusText, y, rowH), col)
		tw := right - sw - 24 - tx
		ty := u.CenterY(titleText, y, rowH)
		if l.progress >= 0 {
			ty -= 6
		}
		u.Text(u.Fit(l.title, titleText, tw), titleText, tx, ty, theme.Paper)
		if l.progress >= 0 {
			bar := gfx.Rect{X: tx, Y: ty + u.LineHeight(titleText) + 4, W: tw, H: barH}
			shape := raster.Shape{W: int(tw), H: barH, Radius: 2.5, Kind: raster.Fill}
			u.Shape(shape, bar, theme.PaperA(0.18))
			u.PushClip(gfx.Rect{X: bar.X, Y: bar.Y, W: tw * l.progress, H: barH})
			u.Shape(shape, bar, u.Accent.Color)
			u.PopClip()
		}
	})
	u.PopClip()
}

// The Retry pill on a failed row: 2 px goldSoft border, the refresh icon.
// retryPill draws the pill, w wide, ending at right.
func retryPill(u *ui.Context, w, right, y float32) {
	x := right - w
	u.Shape(raster.Shape{W: int(w), H: retryH, Radius: retryH / 2, Kind: raster.Ring, RingOffset: -2, RingWidth: 2},
		gfx.Rect{X: x, Y: y, W: w, H: retryH}, theme.GoldSoft)
	u.Icon("refresh", 20, 2, false, x+14, y+(retryH-20)/2, theme.GoldSoft)
	u.Text("Retry", statusText, x+14+20+8, u.CenterY(statusText, y, retryH), theme.GoldSoft)
}

// hints are the hint bar's keys, made again only when they would change.
func (d *Downloads) hints() []widgets.Hint {
	e, ok := d.focused()
	k := hintKey{focus: d.list.Focus(), state: e.State, on: ok, paused: d.status.Paused}
	if d.hintBar != nil && k == d.hintFor {
		return d.hintBar
	}
	d.hintFor = k
	h := make([]widgets.Hint, 0, 4)
	if ok && e.State == media.Failed {
		h = append(h, widgets.Hint{Key: "A", Label: "Retry"})
	}
	if ok {
		h = append(h, widgets.Hint{Key: "X", Label: "Remove"})
	}
	pause := "Pause all"
	if d.status.Paused {
		pause = "Resume all"
	}
	d.hintBar = append(h, widgets.Hint{Key: "Y", Label: pause}, widgets.Hint{Key: "L1/R1", Label: "Tabs"})
	return d.hintBar
}
