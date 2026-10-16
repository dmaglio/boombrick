package browse

import (
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/text"
	"github.com/ujjwal313/boombrick/internal/ui/views"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// The album page: 200 px art in a column on the left with the album's details
// and Play and Shuffle pills at its foot, the tracks on the right.
const (
	albumArt = 200
	albumTop = contentTop + pad
	listX    = side + albumArt + side
	pillGap  = 12
)

const (
	playPill = iota
	shufflePill
	pillCount
)

// AlbumPage shows an album's tracks. Focus starts on the Play pill.
type AlbumPage struct {
	nav    screens.Nav
	p      Player
	album  *library.Album
	tracks []player.Track
	rows   []widgets.Row
	list   views.Grid
	onList bool
	pill   int
	pills  [pillCount]anim.Value // focus strength
}

// NewAlbumPage opens album a.
func NewAlbumPage(nav screens.Nav, p Player, a *library.Album) *AlbumPage {
	pg := &AlbumPage{nav: nav, p: p, album: a, tracks: screens.Tracks(a.Tracks)}
	pg.list = views.Grid{Cols: 1, CellW: theme.ScreenW - side - listX, CellH: widgets.RowHeight, Pad: pad,
		ViewH: contentBot - contentTop}
	pg.list.SetCount(len(a.Tracks))
	pg.list.SetActive(false, time.Time{})
	pg.pills[playPill].Set(1)
	for i, t := range a.Tracks {
		r := widgets.Row{Index: strconv.Itoa(i + 1), Title: t.Title}
		if t.Duration > 0 {
			r.Duration = screens.Clock(t.Duration)
		}
		pg.rows = append(pg.rows, r)
	}
	return pg
}

// Handle moves between the pills and the tracks, plays with A and queues with Y.
func (pg *AlbumPage) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind == input.Release {
		return false
	}
	switch ev.Button {
	case input.A:
		if ev.Kind == input.Press {
			pg.play()
		}
		return ev.Kind == input.Press
	case input.Y:
		if ev.Kind == input.Press {
			pg.enqueue()
		}
		return ev.Kind == input.Press
	}
	if pg.onList {
		if pg.list.Move(ev.Button, now) {
			return true
		}
		if ev.Button != input.Left {
			return false
		}
		pg.onList = false
		pg.list.SetActive(false, now)
		pg.pills[pg.pill].To(1, now, anim.Focus, anim.EaseOutCubic)
		return true
	}
	next := pg.pill
	switch ev.Button {
	case input.Left:
		next--
	case input.Right:
		next++
	default:
		return false
	}
	switch {
	case next == pillCount && pg.list.Count() > 0:
		pg.pills[pg.pill].To(0, now, anim.Focus, anim.EaseOutCubic)
		pg.onList = true
		pg.list.SetActive(true, now)
	case next >= 0 && next < pillCount:
		pg.pills[pg.pill].To(0, now, anim.Focus, anim.EaseOutCubic)
		pg.pill = next
		pg.pills[next].To(1, now, anim.Focus, anim.EaseOutCubic)
	default:
		return false
	}
	return true
}

func (pg *AlbumPage) play() {
	if len(pg.tracks) == 0 {
		return
	}
	switch {
	case pg.onList:
		pg.p.SetShuffle(false)
		pg.p.Play(pg.tracks, pg.list.Focus())
	case pg.pill == playPill:
		pg.p.SetShuffle(false)
		pg.p.Play(pg.tracks, 0)
	default:
		pg.p.SetShuffle(true)
		pg.p.Play(pg.tracks, rand.IntN(len(pg.tracks)))
	}
}

func (pg *AlbumPage) enqueue() {
	if len(pg.tracks) == 0 {
		return
	}
	ts := pg.tracks
	if pg.onList {
		f := pg.list.Focus()
		ts = ts[f : f+1]
	}
	pg.p.Enqueue(ts)
	pg.nav.Toast(screens.AddedToQueue)
}

// Animating reports whether focus or the list is moving.
func (pg *AlbumPage) Animating(now time.Time) bool {
	return pg.list.Animating(now) || !pg.pills[playPill].Done(now) || !pg.pills[shufflePill].Done(now)
}

// Draw renders the frame.
func (pg *AlbumPage) Draw(u *ui.Context) {
	now := u.Now
	a := pg.album
	widgets.DrawBackground(u, widgets.TabGround)

	widgets.Art(u, screens.AlbumArt(a), albumArt, widgets.CardRadius, gfx.Rect{X: side, Y: albumTop, W: albumArt, H: albumArt})
	y := float32(albumTop + albumArt + pad)
	u.Text(u.Fit(a.Title, text.Heading, albumArt), text.Heading, side, y, theme.Paper)
	y += u.LineHeight(text.Heading)
	who := a.Artist
	if a.Year > 0 {
		who += " · " + strconv.Itoa(a.Year)
	}
	u.Text(u.Fit(who, text.Meta, albumArt), text.Meta, side, y, theme.Ink2)
	y += u.LineHeight(text.Meta)
	u.Text(u.Fit(screens.SongsAndTime(len(a.Tracks), a.Duration), text.Meta, albumArt), text.Meta, side, y, theme.Ink3)
	py := float32(contentBot - pad - widgets.PillHeight)
	x := float32(side)
	play, shuffle := float32(pg.pills[playPill].At(now)), float32(pg.pills[shufflePill].At(now))
	widgets.PillGlow(u, "play", "Play", x, py, play)
	x += widgets.Pill(u, "play", "Play", x, py, play) + pillGap
	widgets.PillGlow(u, "shuffle", "", x, py, shuffle)
	widgets.Pill(u, "shuffle", "", x, py, shuffle)

	playing := pg.p.State().Track.Path
	u.PushClip(glowArea)
	pg.list.Draw(now, listX, contentTop, func(i int, x, y, k float32) {
		r := pg.rows[i]
		r.Playing = pg.tracks[i].Path == playing
		box := gfx.Rect{X: x, Y: y, W: pg.list.CellW, H: widgets.RowHeight}
		widgets.RowGlow(u, box, widgets.RowRadius, k)
		u.PushClip(below(contentTop))
		widgets.ListRow(u, r, box, k)
		u.PopClip()
	})
	u.PopClip()

	widgets.StatusBar(u)
	widgets.Tabs(u, 1)
	widgets.HintBar(u, pg.hints())
}

func (pg *AlbumPage) hints() []widgets.Hint {
	a := "Play"
	if !pg.onList && pg.pill == shufflePill {
		a = "Shuffle"
	}
	return []widgets.Hint{{Key: "A", Label: a}, {Key: "Y", Label: "Add to queue"}, {Key: "B", Label: "Back"},
		{Key: "L1/R1", Label: "Tabs"}}
}
