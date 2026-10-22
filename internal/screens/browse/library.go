package browse

import (
	"slices"
	"time"

	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/resume"
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
	booksFilter
	filterCount
)

var filterNames = [filterCount]string{"Albums", "Artists", "Songs", "Books"}

const listTop = contentTop + widgets.FiltersHeight

const emptyText = "Copy music to the Music folder on the SD card, or find something on YouTube Music. Audiobooks (.m4b) go in Music/Audiobooks."

// Library is the Library tab: albums, artists or songs, chosen with L2 and R2.
type Library struct {
	nav      screens.Nav
	p        Player
	musicDir string
	rescan   func()
	ix       *library.Index
	filter   int
	views    [filterCount]views.Grid
	counts   [filterCount]string    // the filters' counts
	rows     [filterCount][]listRow // the artists' and songs' rows, made as they first show
	laidOut  bool
	progress []resume.Progress // the albums and books part-way through
	remote   Remote            // asks where else a book was left; nil without Audible
	reading  bool              // the first scan is running with nothing indexed yet
	state    widgets.Wrapped   // the empty or reading state's description, wrapped
}

// listRow is what an artist's or a song's row shows that takes making.
type listRow struct {
	art      widgets.ArtSpec
	subtitle string
	duration string // a song's length, or ""
}

// New creates the tab. It shows nothing but its chrome until SetIndex.
func New(nav screens.Nav, p Player, musicDir string, rescan func()) *Library {
	l := &Library{nav: nav, p: p, musicDir: musicDir, rescan: rescan}
	l.views[albumsFilter] = screens.AlbumGrid()
	l.views[booksFilter] = screens.AlbumGrid()
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
	l.counts = [filterCount]string{screens.Count(len(ix.Albums)), screens.Count(len(ix.Artists)), screens.Count(len(ix.Songs)), screens.Count(len(ix.Books))}
	l.rows[artistsFilter] = make([]listRow, len(ix.Artists))
	l.rows[songsFilter] = make([]listRow, len(ix.Songs))
	if old == nil {
		l.views[albumsFilter].SetCount(len(ix.Albums))
		l.views[artistsFilter].SetCount(len(ix.Artists))
		l.views[songsFilter].SetCount(len(ix.Songs))
		l.views[booksFilter].SetCount(len(ix.Books))
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
	g = &l.views[booksFilter]
	g.SetCountFocus(len(ix.Books), refocus(old.Books, ix.Books, g.Focus(),
		func(a *library.Album) [2]string { return [2]string{a.Artist, a.Title} }))
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

// Remote asks whether a book was left further on somewhere else, such as on
// the phone, than where the card left it: localFrame as of localWhen, if
// hasLocal. When it was, it gives that place's frame and true. It may take a
// few seconds, and is called off the UI thread.
type Remote func(path string, localFrame int64, localWhen time.Time, hasLocal bool) (frame int64, ok bool)

// SetRemote makes books check where they were left elsewhere before they
// start. A nil r turns it off.
func (l *Library) SetRemote(r Remote) { l.remote = r }

// SetProgress tells the tab which albums and books are part-way through, for
// a book to carry on from where it was left.
func (l *Library) SetProgress(p []resume.Progress) { l.progress = p }

// SetReading says the first scan is running and nothing is indexed yet;
// until SetIndex the tab says the music is being read.
func (l *Library) SetReading() { l.reading = true }

func (l *Library) empty() bool { return l.ix != nil && len(l.ix.Albums) == 0 && len(l.ix.Books) == 0 }

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
	case booksFilter:
		l.p.SetShuffle(false)
		book := l.ix.Books[i]
		track, frame, when, has := l.placeIn(book)
		tracks := screens.Tracks(book.Tracks)
		if l.remote == nil || len(tracks) != 1 {
			l.p.PlayFrom(tracks, track, frame)
			break
		}
		remote, p := l.remote, l.p
		go func() { // asking Audible takes a moment; the card's place stands if it can't
			if f, ok := remote(tracks[0].Path, frame, when, has); ok {
				frame = f
			}
			p.PlayFrom(tracks, track, frame)
		}()
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
		ar := l.ix.Artists[i]
		ts := make([]player.Track, 0, ar.Songs)
		for _, a := range ar.Albums {
			for j := range a.Tracks {
				ts = append(ts, screens.TrackOf(&a.Tracks[j]))
			}
		}
		return ts
	}
	if l.filter == booksFilter {
		return screens.Tracks(l.ix.Books[i].Tracks)
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
	widgets.Filters(u, filterNames[:], l.counts[:], l.filter, side, contentTop)
}

// row is item i's row in the artists or songs, made the first time it shows
// rather than every frame.
func (l *Library) row(i int) *listRow {
	r := &l.rows[l.filter][i]
	if r.subtitle != "" {
		return r
	}
	if l.filter == artistsFilter {
		ar := l.ix.Artists[i]
		r.art, r.subtitle = screens.ArtistArt(ar), artistLine(len(ar.Albums), ar.Songs)
		return r
	}
	s := l.ix.Songs[i]
	r.art, r.subtitle = screens.SongArt(s), s.Artist+" · "+s.Album
	if s.Duration > 0 {
		r.duration = screens.Clock(s.Duration)
	}
	return r
}

func (l *Library) drawView(u *ui.Context) {
	g := &l.views[l.filter]
	if l.filter == albumsFilter {
		drawAlbums(u, g, l.ix.Albums, listTop, func(i int) string { return l.ix.Albums[i].Artist })
		return
	}
	if l.filter == booksFilter {
		drawAlbums(u, g, l.ix.Books, listTop, func(i int) string { return l.ix.Books[i].Artist })
		return
	}
	playing := l.p.State().Track.Path
	u.PushClip(screens.GlowArea)
	g.Draw(u.Now, side, listTop, func(i int, x, y, k float32) {
		lr := l.row(i)
		r := widgets.Row{Art: &lr.art, Subtitle: lr.subtitle, Duration: lr.duration}
		if l.filter == artistsFilter {
			r.Title, r.Round = l.ix.Artists[i].Name, true
		} else {
			s := l.ix.Songs[i]
			r.Title, r.Playing = s.Title, s.Path == playing
		}
		box := gfx.Rect{X: x, Y: y, W: g.CellW, H: widgets.RowHeight}
		widgets.RowGlow(u, box, widgets.RowRadius, k)
		u.PushClip(screens.Below(listTop))
		widgets.ListRow(u, r, box, k)
		u.PopClip()
	})
	u.PopClip()
}

// The tab's hint bars: before the first index, for an empty library, and
// with albums or artists, or songs, shown.
var (
	tabHints    = []widgets.Hint{{Key: "L1/R1", Label: "Tabs"}}
	rescanHints = []widgets.Hint{{Key: "Y", Label: "Rescan SD card"}, {Key: "L1/R1", Label: "Tabs"}}
	openHints   = []widgets.Hint{{Key: "A", Label: "Open"}, {Key: "Y", Label: "Add to queue"},
		{Key: "L2/R2", Label: "Filter"}, {Key: "L1/R1", Label: "Tabs"}}
	songHints = []widgets.Hint{{Key: "A", Label: "Play"}, {Key: "Y", Label: "Add to queue"},
		{Key: "L2/R2", Label: "Filter"}, {Key: "L1/R1", Label: "Tabs"}}
)

func (l *Library) hints() []widgets.Hint {
	switch {
	case l.ix == nil:
		return tabHints
	case l.empty():
		return rescanHints
	case l.filter == songsFilter, l.filter == booksFilter:
		return songHints
	}
	return openHints
}

// placeIn is the track and frame to carry on a book from: where it was left
// if it is part-way through, else the start, with when it was left (has is
// false at the start).
func (l *Library) placeIn(a *library.Album) (track int, frame int64, when time.Time, has bool) {
	for _, pr := range l.progress {
		if pr.Artist != a.Artist || pr.Album != a.Title {
			continue
		}
		if i := slices.IndexFunc(a.Tracks, func(t library.Track) bool { return t.Path == pr.Path }); i >= 0 {
			return i, pr.Frame, pr.Updated, true
		}
	}
	return 0, 0, time.Time{}, false
}
