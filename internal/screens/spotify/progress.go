package spotify

import (
	"strconv"
	"time"

	"github.com/ujjwal313/boombrick/internal/importer"
	"github.com/ujjwal313/boombrick/internal/media"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/spotify"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
	"github.com/ujjwal313/boombrick/internal/ui/text"
	"github.com/ujjwal313/boombrick/internal/ui/views"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
	"github.com/ujjwal313/boombrick/internal/ytmusic"
)

// The downloading page's summary: the playlist's art, a big counter and a
// bar; then its songs' rows.
const (
	summaryArt  = 120
	barH        = 10
	stateSize   = 40
	progressRow = 72
)

var (
	counterText = text.Style{Face: text.Lora, Size: 56, Tabular: true} // Cormorant's old-style 1 reads as I
	ofText      = text.Style{Face: text.Lora, Size: 26}
	lineText    = text.Style{Face: text.Lora, Size: 22, Tabular: true}
	statusText  = text.Style{Face: text.Lora, Size: 20, Tabular: true}
)

var (
	progressHints = []widgets.Hint{{Key: "B", Label: "Run in background"}, {Key: "Y", Label: "Pause"}, {Key: "X", Label: "Cancel import"}}
	pausedHints   = []widgets.Hint{{Key: "B", Label: "Run in background"}, {Key: "Y", Label: "Resume"}, {Key: "X", Label: "Cancel import"}}
)

// Progress follows the downloads of an import's songs, those matched and
// accepted. Leaving it leaves the import's pages, which run on in the
// background.
type Progress struct {
	e       *Env
	pls     []spotify.Playlist
	version uint64
	songs   []ytmusic.Song // the songs downloading, in the playlists' order
	labels  []string       // their rows' "Title — Artist"
	list    views.Grid
	art     widgets.ArtSpec
	msg     widgets.Wrapped
	counted [2]int // saved and all, as last drawn
	counter string
	of      string
}

// NewProgress follows the downloads of pls' imports.
func NewProgress(e *Env, pls []spotify.Playlist) *Progress {
	p := &Progress{e: e, pls: pls,
		list: views.Grid{Cols: 1, CellW: theme.ScreenW - 2*screens.Side, CellH: progressRow, Pad: screens.Pad}}
	if len(pls) > 0 {
		p.art = art(pls[0].Name, pls[0].Cover)
		if pls[0].URI == spotify.LikedURI {
			p.art = screens.LikedArt
		}
	}
	p.version = ^uint64(0)
	return p
}

// ShowsDownloads says the page follows the download queue.
func (p *Progress) ShowsDownloads() bool { return true }

// refresh takes the songs to follow when an import has changed.
func (p *Progress) refresh() {
	v := p.e.Import.Version()
	if v == p.version {
		return
	}
	p.version = v
	p.songs, p.labels = p.songs[:0], p.labels[:0]
	for _, pl := range p.pls {
		job, ok := p.e.Import.Job(pl.URI)
		if !ok {
			continue
		}
		for _, it := range job.Items {
			if it.State != importer.Matched && it.State != importer.Accepted {
				continue
			}
			s := it.Candidates[it.Chosen].Song
			p.songs = append(p.songs, s)
			p.labels = append(p.labels, it.Track.Title+" — "+s.Artist)
		}
	}
	p.list.SetCountFocus(len(p.songs), min(p.list.Focus(), max(len(p.songs)-1, 0)))
}

// Handle pauses or resumes the download queue with Y, and cancels the
// import with X.
func (p *Progress) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind != input.Press {
		return p.list.Move(ev.Button, now)
	}
	switch ev.Button {
	case input.Y:
		p.e.Queue.SetPaused(!p.e.Queue.Paused())
		return true
	case input.X:
		for _, pl := range p.pls {
			p.e.Import.Cancel(pl.URI)
		}
		p.e.Nav.Toast("Import cancelled")
		p.e.leaving = true
		p.e.Nav.Pop()
		return true
	}
	return p.list.Move(ev.Button, now)
}

// Back leaves the import's pages: B pops this one, and the others pop
// themselves.
func (p *Progress) Back() bool {
	p.e.leaving = true
	return false
}

// Animating reports whether the list is moving.
func (p *Progress) Animating(now time.Time) bool { return p.list.Animating(now) }

// Draw renders the frame.
func (p *Progress) Draw(u *ui.Context) {
	p.refresh()
	widgets.DrawBackground(u, widgets.FlatGround)
	widgets.StatusBar(u)
	title := "Downloading"
	if len(p.pls) == 1 {
		title = p.pls[0].Name
	}
	top := importHeader(u, title, 2)
	saved, done := 0, float32(0)
	for _, s := range p.songs {
		st, f := p.e.Queue.State(s.VideoID)
		switch st {
		case media.Saved:
			saved++
			done++
		case media.Downloading:
			done += f
		}
	}
	if c := [2]int{saved, len(p.songs)}; c != p.counted || p.counter == "" {
		p.counted, p.counter, p.of = c, strconv.Itoa(saved), "of "+songs(len(p.songs))
	}
	x := float32(screens.Side)
	widgets.Art(u, p.art, summaryArt, widgets.CardRadius, gfx.Rect{X: x, Y: top, W: summaryArt, H: summaryArt})
	sx := x + summaryArt + 24
	u.Text(p.counter, counterText, sx, top-8, theme.Paper)
	u.Text(p.of, ofText, sx+u.TextWidth(p.counter, counterText)+12, top+u.Ascent(counterText)-u.Ascent(ofText)-8, theme.Ink2)
	by := top + u.LineHeight(counterText) - 4
	bw := theme.ScreenW - screens.Side - sx
	bar := raster.Shape{W: int(bw), H: barH, Radius: barH / 2, Kind: raster.Fill}
	u.Shape(bar, gfx.Rect{X: sx, Y: by, W: bw, H: barH}, theme.PaperA(0.18))
	if len(p.songs) > 0 && done > 0 {
		u.PushClip(gfx.Rect{X: sx, Y: by, W: bw * done / float32(len(p.songs)), H: barH})
		u.Shape(bar, gfx.Rect{X: sx, Y: by, W: bw, H: barH}, u.Accent.Color)
		u.PopClip()
	}
	line := "Downloading"
	switch {
	case p.e.Queue.Paused():
		line = "Paused"
	case saved == len(p.songs) && len(p.songs) > 0:
		line = "Done"
	case p.e.Queue.Note() != "":
		line = p.e.Queue.Note()
	}
	u.Text(line, lineText, sx, by+barH+12, theme.Ink2)
	top += summaryArt + 20
	if len(p.songs) == 0 {
		message(u, &p.msg, "No songs to download yet.", gfx.Rect{X: screens.Side, Y: top, W: theme.ScreenW - 2*screens.Side,
			H: screens.ContentBot - top})
	} else {
		p.drawRows(u, top)
	}
	hints := progressHints
	if p.e.Queue.Paused() {
		hints = pausedHints
	}
	widgets.HintBar(u, hints)
}

func (p *Progress) drawRows(u *ui.Context, top float32) {
	p.list.ViewH = screens.ContentBot - top
	u.PushClip(gfx.Rect{X: 0, Y: top, W: theme.ScreenW, H: screens.ContentBot - top})
	p.list.Draw(u.Now, screens.Side, top, func(i int, x, y, k float32) {
		s := p.songs[i]
		st, f := p.e.Queue.State(s.VideoID)
		widgets.DownloadCircle(u, st, f, stateSize, x, y+(progressRow-stateSize)/2)
		status := downloadText(st, f)
		sw := u.TextWidth(status, statusText)
		u.Text(status, statusText, x+p.list.CellW-sw, u.CenterY(statusText, y, progressRow), theme.Ink2)
		tx := x + stateSize + 16
		u.Text(u.Fit(p.labels[i], matchTitle, p.list.CellW-stateSize-16-sw-16), matchTitle, tx, u.CenterY(matchTitle, y, progressRow), theme.Paper)
		u.Fill(gfx.Rect{X: x, Y: y + progressRow - 1, W: p.list.CellW, H: 1}, theme.PaperA(0.08))
	})
	u.PopClip()
}

// downloadText is a song's download as its row says it.
func downloadText(st media.DownloadState, f float32) string {
	switch st {
	case media.Downloading:
		return strconv.Itoa(int(f*100+0.5)) + " %"
	case media.Saved:
		return "Saved"
	case media.Failed:
		return "Failed"
	}
	return "Queued"
}
