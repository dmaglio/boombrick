package browse

import (
	"slices"
	"time"

	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/views"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

const (
	albumsFilter = iota
	artistsFilter
	songsFilter
	filterCount
)

var filterNames = [filterCount]string{"Albums", "Artists", "Songs"}

const listTop = contentTop + widgets.FiltersHeight

const emptyText = "Copy music to the Music folder on the SD card, or find something on YouTube Music."

// Library is the Library tab: albums, artists or songs, chosen with L2 and R2.
type Library struct {
	nav      screens.Nav
	p        Player
	musicDir string
	rescan   func()
	ix       *library.Index
	filter   int
	views    [filterCount]views.Grid
	laidOut  bool
	reading  bool            // the first scan is running with nothing indexed yet
	state    widgets.Wrapped // the empty or reading state's description, wrapped
}

// New creates the tab. It shows nothing but its chrome until SetIndex.
func New(nav screens.Nav, p Player, musicDir string, rescan func()) *Library {
	l := &Library{nav: nav, p: p, musicDir: musicDir, rescan: rescan}
	l.views[albumsFilter] = screens.AlbumGrid()
	for _, f := range []int{artistsFilter, songsFilter} {
		l.views[f] = views.Grid{Cols: 1, CellW: theme.ScreenW - 2*side, CellH: widgets.RowHeight, Pad: pad,
			ViewH: contentBot - listTop}
	}
	return l
}

// SetIndex shows ix, keeping each filter's focus on the same album, artist or
// song when it is still there.
func (l *Library) SetIndex(ix *library.Index) {
	old := l.ix
	l.ix = ix
	if old == nil {
		l.views[albumsFilter].SetCount(len(ix.Albums))
		l.views[artistsFilter].SetCount(len(ix.Artists))
		l.views[songsFilter].SetCount(len(ix.Songs))
		return
	}
	g := &l.views[albumsFilter]
	g.SetCountFocus(len(ix.Albums), refocus(old.Albums, ix.Albums, g.Focus(),
		func(a *library.Album) [2]string { return [2]string{a.Artist, a.Title} }))
	g = &l.views[artistsFilter]
	g.SetCountFocus(len(ix.Artists), refocus(old.Artists, ix.Artists, g.Focus(),
		func(a *library.Artist) string { return a.Name }))
	g = &l.views[songsFilter]
	g.SetCountFocus(len(ix.Songs), refocus(old.Songs, ix.Songs, g.Focus(),
		func(s *library.Track) string { return s.Path }))
}

// refocus finds the item that was at f in old within now, by key; without it
// focus keeps its position.
func refocus[T any, K comparable](old, now []T, f int, key func(T) K) int {
	if f < 0 || f >= len(old) {
		return f
	}
	k := key(old[f])
	if i := slices.IndexFunc(now, func(x T) bool { return key(x) == k }); i >= 0 {
		return i
	}
	return f
}

// SetReading says the first scan is running and nothing is indexed yet;
// until SetIndex the tab says the music is being read.
func (l *Library) SetReading() { l.reading = true }

func (l *Library) empty() bool { return l.ix != nil && len(l.ix.Albums) == 0 }

// Handle switches filters with L2/R2, moves with the D-pad, opens or plays
// with A and queues with Y. The empty library rescans with Y.
func (l *Library) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind == input.Release || l.ix == nil {
		return false
	}
	if l.empty() {
		if ev.Kind == input.Press && ev.Button == input.Y && l.rescan != nil {
			l.rescan()
			l.nav.Toast(screens.Scanning)
			return true
		}
		return false
	}
	g := &l.views[l.filter]
	if ev.Kind == input.Press {
		switch ev.Button {
		case input.L2, input.R2:
			f := l.filter - 1
			if ev.Button == input.R2 {
				f = l.filter + 1
			}
			if f < 0 || f >= filterCount {
				return false
			}
			l.filter = f
			return true
		case input.A:
			if g.Count() > 0 {
				l.open(g.Focus())
			}
			return true
		case input.Y:
			if g.Count() > 0 {
				l.p.Enqueue(l.tracksAt(g.Focus()))
				l.nav.Toast(screens.AddedToQueue)
			}
			return true
		}
	}
	return g.Move(ev.Button, now)
}

func (l *Library) open(i int) {
	switch l.filter {
	case albumsFilter:
		l.nav.Push(NewAlbumPage(l.nav, l.p, l.ix.Albums[i]))
	case artistsFilter:
		l.nav.Push(NewArtistPage(l.nav, l.p, l.ix.Artists[i]))
	case songsFilter:
		songs := make([]player.Track, len(l.ix.Songs))
		for j, s := range l.ix.Songs {
			songs[j] = screens.TrackOf(s)
		}
		l.p.SetShuffle(false)
		l.p.Play(songs, i)
	}
}

// tracksAt is what Y queues for item i of the current filter.
func (l *Library) tracksAt(i int) []player.Track {
	switch l.filter {
	case albumsFilter:
		return screens.Tracks(l.ix.Albums[i].Tracks)
	case artistsFilter:
		var ts []player.Track
		for _, a := range l.ix.Artists[i].Albums {
			ts = append(ts, screens.Tracks(a.Tracks)...)
		}
		return ts
	}
	return []player.Track{screens.TrackOf(l.ix.Songs[i])}
}

// Animating reports whether the current view is moving.
func (l *Library) Animating(now time.Time) bool { return l.views[l.filter].Animating(now) }

// Draw renders the frame.
func (l *Library) Draw(u *ui.Context) {
	if !l.laidOut {
		l.laidOut = true
		screens.LayoutGrid(u, &l.views[albumsFilter], contentBot-listTop)
	}
	switch {
	case l.empty():
		widgets.DrawBackground(u, widgets.StateGround)
		widgets.EmptyState(u, &l.state, "Your library is empty", emptyText, l.musicDir)
	case l.ix == nil && l.reading:
		screens.DrawReading(u, &l.state, l.musicDir)
	default:
		widgets.DrawBackground(u, widgets.TabGround)
		if l.ix != nil {
			l.drawFilters(u)
			l.drawView(u)
		}
	}
	widgets.StatusBar(u)
	widgets.Tabs(u, 1)
	widgets.HintBar(u, l.hints())
}

// drawFilters draws L2, the three filters with their counts, and R2.
func (l *Library) drawFilters(u *ui.Context) {
	counts := []string{screens.Count(len(l.ix.Albums)), screens.Count(len(l.ix.Artists)), screens.Count(len(l.ix.Songs))}
	widgets.Filters(u, filterNames[:], counts, l.filter, side, contentTop)
}

func (l *Library) drawView(u *ui.Context) {
	g := &l.views[l.filter]
	if l.filter == albumsFilter {
		drawAlbums(u, g, l.ix.Albums, listTop, func(a *library.Album) string { return a.Artist })
		return
	}
	playing := l.p.State().Track.Path
	u.PushClip(screens.GlowArea)
	g.Draw(u.Now, side, listTop, func(i int, x, y, k float32) {
		var r widgets.Row
		if l.filter == artistsFilter {
			ar := l.ix.Artists[i]
			art := screens.ArtistArt(ar)
			r = widgets.Row{Art: &art, Round: true, Title: ar.Name, Subtitle: artistLine(len(ar.Albums), ar.Songs)}
		} else {
			s := l.ix.Songs[i]
			art := screens.SongArt(s)
			r = widgets.Row{Art: &art, Title: s.Title, Subtitle: s.Artist + " · " + s.Album, Playing: s.Path == playing}
			if s.Duration > 0 {
				r.Duration = screens.Clock(s.Duration)
			}
		}
		box := gfx.Rect{X: x, Y: y, W: g.CellW, H: widgets.RowHeight}
		widgets.RowGlow(u, box, widgets.RowRadius, k)
		u.PushClip(screens.Below(listTop))
		widgets.ListRow(u, r, box, k)
		u.PopClip()
	})
	u.PopClip()
}

func (l *Library) hints() []widgets.Hint {
	switch {
	case l.ix == nil:
		return []widgets.Hint{{Key: "L1/R1", Label: "Tabs"}}
	case l.empty():
		return []widgets.Hint{{Key: "Y", Label: "Rescan SD card"}, {Key: "L1/R1", Label: "Tabs"}}
	}
	open := "Open"
	if l.filter == songsFilter {
		open = "Play"
	}
	return []widgets.Hint{{Key: "A", Label: open}, {Key: "Y", Label: "Add to queue"}, {Key: "L2/R2", Label: "Filter"},
		{Key: "L1/R1", Label: "Tabs"}}
}
