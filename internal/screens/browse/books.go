package browse

import (
	"slices"
	"time"

	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/resume"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/views"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// BooksTab is the Books tab's place in the tab row.
const BooksTab = 2

const booksEmptyText = "Copy audiobooks (.m4b files) to Music/Audiobooks on the SD card."

// Remote asks whether a book was left further on somewhere else, such as on
// the phone, than where the card left it: localFrame as of localWhen, if
// hasLocal. When it was, it gives that place's frame and true. It may take a
// few seconds, and is called off the UI thread.
type Remote func(path string, localFrame int64, localWhen time.Time, hasLocal bool) (frame int64, ok bool)

// Books is the Books tab: the audiobooks in the library as a grid of covers.
// A plays a book from where it was left, Y adds it to the queue.
type Books struct {
	nav      screens.Nav
	p        Player
	musicDir string
	ix       *library.Index
	view     views.Grid
	progress []resume.Progress // the albums and books part-way through
	remote   Remote            // asks where else a book was left; nil without Audible
	laidOut  bool
	state    widgets.Wrapped // the empty state's description, wrapped
}

// NewBooks creates the tab. It shows nothing but its chrome until SetIndex.
func NewBooks(nav screens.Nav, p Player, musicDir string) *Books {
	return &Books{nav: nav, p: p, musicDir: musicDir, view: screens.AlbumGrid()}
}

// SetIndex shows the books of ix, keeping focus on the same book when it is
// still there.
func (b *Books) SetIndex(ix *library.Index) {
	old := b.ix
	b.ix = ix
	if old == nil {
		b.view.SetCount(len(ix.Books))
		return
	}
	b.view.SetCountFocus(len(ix.Books), refocus(old.Books, ix.Books, b.view.Focus(),
		func(a *library.Album) [2]string { return [2]string{a.Artist, a.Title} }))
}

// SetProgress tells the tab which albums and books are part-way through, for
// a book to carry on from where it was left.
func (b *Books) SetProgress(p []resume.Progress) { b.progress = p }

// SetRemote makes books check where they were left elsewhere before they
// start. A nil r turns it off.
func (b *Books) SetRemote(r Remote) { b.remote = r }

func (b *Books) empty() bool { return b.ix != nil && len(b.ix.Books) == 0 }

// Handle moves with the D-pad, plays a book with A and queues it with Y.
func (b *Books) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind == input.Release || b.ix == nil || b.empty() {
		return false
	}
	if ev.Kind == input.Press {
		switch ev.Button {
		case input.A:
			b.play(b.view.Focus())
			return true
		case input.Y:
			b.p.Enqueue(screens.Tracks(b.ix.Books[b.view.Focus()].Tracks))
			b.nav.Toast(screens.AddedToQueue)
			return true
		}
	}
	return b.view.Move(ev.Button, now)
}

// play starts book i from where it was left; with Audible, from where it was
// left furthest.
func (b *Books) play(i int) {
	book := b.ix.Books[i]
	b.p.SetShuffle(false)
	track, frame, when, has := b.placeIn(book)
	tracks := screens.Tracks(book.Tracks)
	if b.remote == nil || len(tracks) != 1 {
		b.p.PlayFrom(tracks, track, frame)
		return
	}
	remote, p := b.remote, b.p
	go func() { // asking Audible takes a moment; the card's place stands if it can't
		if f, ok := remote(tracks[0].Path, frame, when, has); ok {
			frame = f
		}
		p.PlayFrom(tracks, track, frame)
	}()
}

// placeIn is the track and frame to carry on a book from: where it was left
// if it is part-way through, else the start, with when it was left (has is
// false at the start).
func (b *Books) placeIn(a *library.Album) (track int, frame int64, when time.Time, has bool) {
	for _, pr := range b.progress {
		if pr.Artist != a.Artist || pr.Album != a.Title {
			continue
		}
		if i := slices.IndexFunc(a.Tracks, func(t library.Track) bool { return t.Path == pr.Path }); i >= 0 {
			return i, pr.Frame, pr.Updated, true
		}
	}
	return 0, 0, time.Time{}, false
}

// Animating reports whether the grid is moving.
func (b *Books) Animating(now time.Time) bool { return b.view.Animating(now) }

// Draw renders the frame.
func (b *Books) Draw(u *ui.Context) {
	if !b.laidOut {
		b.laidOut = true
		screens.LayoutGrid(u, &b.view, contentBot-contentTop)
	}
	switch {
	case b.empty():
		widgets.DrawBackground(u, widgets.StateGround)
		widgets.EmptyState(u, &b.state, "No audiobooks yet", booksEmptyText, b.musicDir)
	default:
		widgets.DrawBackground(u, widgets.TabGround)
		if b.ix != nil {
			drawAlbums(u, &b.view, b.ix.Books, contentTop, func(i int) string { return b.ix.Books[i].Artist })
		}
	}
	widgets.StatusBar(u)
	widgets.Tabs(u, BooksTab)
	widgets.HintBar(u, b.hints())
}

var (
	booksPlayHints = []widgets.Hint{{Key: "A", Label: "Play"}, {Key: "Y", Label: "Add to queue"},
		{Key: "L1/R1", Label: "Tabs"}}
	booksEmptyHints = []widgets.Hint{{Key: "L1/R1", Label: "Tabs"}}
)

func (b *Books) hints() []widgets.Hint {
	if b.ix == nil || b.empty() {
		return booksEmptyHints
	}
	return booksPlayHints
}
