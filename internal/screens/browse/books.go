package browse

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/ujjwal313/boombrick/internal/audiobooks"
	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/media"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/resume"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/views"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// BooksTab is the Books tab's place in the tab row.
const BooksTab = 2

// Catalogue is the listener's Audible library, as the Books tab shows it and
// drives it. *audiobooks.Service is one.
type Catalogue interface {
	SignedIn() bool
	Status() (audiobooks.Status, string)
	Entries() []audiobooks.Entry
	Download(asin string)
	Refresh()
}

// Remote asks whether a book was left further on somewhere else, such as on
// the phone, than where the card left it: localFrame as of localWhen, if
// hasLocal. When it was, it gives that place's frame and true. It may take a
// few seconds, and is called off the UI thread.
type Remote func(path string, localFrame int64, localWhen time.Time, hasLocal bool) (frame int64, ok bool)

// Books is the Books tab: the books of the listener's Audible library. A
// downloads a book that isn't on the card yet, and plays one that is, from
// where it was left; in the Library it is found like any other.
type Books struct {
	nav     screens.Nav
	p       Player
	shelf   *Shelf
	cat     Catalogue
	setup   string // where the Audible registration goes, for the tab to say
	ix      *library.Index
	entries []audiobooks.Entry
	view    views.Grid
	state   widgets.Wrapped // an empty state's description, wrapped
}

// NewBooks creates the tab. setup is the file the Audible registration is
// read from, which the tab names when there is none.
func NewBooks(nav screens.Nav, p Player, shelf *Shelf, cat Catalogue, setup string) *Books {
	return &Books{nav: nav, p: p, shelf: shelf, cat: cat, setup: setup,
		view: views.Grid{Cols: 1, CellW: theme.ScreenW - 2*side, CellH: widgets.RowHeight, Pad: pad, ViewH: contentBot - contentTop}}
}

// SetIndex tells the tab what the library holds, to play the books on the card.
func (b *Books) SetIndex(ix *library.Index) { b.ix = ix }

// sync takes the catalogue's books, keeping focus on the same one.
func (b *Books) sync() {
	b.entries = b.cat.Entries()
	if b.view.Count() != len(b.entries) {
		b.view.SetCount(len(b.entries))
	}
}

// Handle moves with the D-pad, downloads or plays a book with A, adds one on
// the card to the queue with Y, and asks for the list again with X.
func (b *Books) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind == input.Release {
		return false
	}
	b.sync()
	if ev.Kind == input.Press {
		switch ev.Button {
		case input.X:
			if b.cat.SignedIn() {
				b.cat.Refresh()
				b.nav.Toast("Reading your Audible library")
			}
			return true
		case input.A, input.Y:
			if len(b.entries) == 0 {
				return false
			}
			b.act(b.entries[b.view.Focus()], ev.Button == input.Y)
			return true
		}
	}
	return b.view.Move(ev.Button, now)
}

// act does what A (or Y, to queue) means for the book e.
func (b *Books) act(e audiobooks.Entry, queue bool) {
	switch {
	case e.Path != "":
		book := b.onCard(e.Path)
		if book == nil {
			b.nav.Toast(screens.Scanning)
			return
		}
		if queue {
			b.p.Enqueue(screens.Tracks(book.Tracks))
			b.nav.Toast(screens.AddedToQueue)
			return
		}
		b.shelf.Play(book)
	case queue:
	case e.State == audiobooks.Queued || e.State == audiobooks.Downloading || e.State == audiobooks.Decrypting:
		b.nav.Toast("Already on its way")
	default:
		b.cat.Download(e.ASIN)
		b.nav.Toast("Downloading " + e.Title)
	}
}

// onCard finds the book at path in the library.
func (b *Books) onCard(path string) *library.Album {
	if b.ix == nil {
		return nil
	}
	for _, a := range b.ix.Books {
		if slices.ContainsFunc(a.Tracks, func(t library.Track) bool { return t.Path == path }) {
			return a
		}
	}
	return nil
}

// Shelf plays books: from where they were left, here or, with Audible, on
// another device, whichever is further on. The Library and the Books tab share
// one.
type Shelf struct {
	p        Player
	progress []resume.Progress // the albums and books part-way through
	remote   Remote            // asks where else a book was left; nil without Audible
}

// NewShelf makes a shelf that plays through p.
func NewShelf(p Player) *Shelf { return &Shelf{p: p} }

// SetProgress tells the shelf which albums and books are part-way through.
func (sh *Shelf) SetProgress(p []resume.Progress) { sh.progress = p }

// SetRemote makes books check where they were left elsewhere before they
// start. A nil r turns it off.
func (sh *Shelf) SetRemote(r Remote) { sh.remote = r }

// Play starts book from where it was left.
func (sh *Shelf) Play(book *library.Album) {
	sh.p.SetShuffle(false)
	track, frame, when, has := sh.placeIn(book)
	tracks := screens.Tracks(book.Tracks)
	if sh.remote == nil || len(tracks) != 1 {
		sh.p.PlayFrom(tracks, track, frame)
		return
	}
	remote, p := sh.remote, sh.p
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
func (sh *Shelf) placeIn(a *library.Album) (track int, frame int64, when time.Time, has bool) {
	for _, pr := range sh.progress {
		if pr.Artist != a.Artist || pr.Album != a.Title {
			continue
		}
		if i := slices.IndexFunc(a.Tracks, func(t library.Track) bool { return t.Path == pr.Path }); i >= 0 {
			return i, pr.Frame, pr.Updated, true
		}
	}
	return 0, 0, time.Time{}, false
}

// Animating reports whether the list is moving or a book is on its way, whose
// progress shows.
func (b *Books) Animating(now time.Time) bool { return b.view.Animating(now) }

// length is a book's length as people say it: "4 h 9 min".
func length(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	switch {
	case m <= 0:
		return ""
	case m < 60:
		return strconv.Itoa(m) + " min"
	case m%60 == 0:
		return strconv.Itoa(m/60) + " h"
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}

// rowOf is what a book's row shows.
func rowOf(e audiobooks.Entry, playing string) widgets.Row {
	spec := widgets.SpecFor(e.Title)
	if e.Cover != "" {
		spec.Cover = media.Cover{URL: e.Cover}
	}
	sub := e.Authors
	if l := length(e.Length); l != "" {
		sub += " · " + l
	}
	r := widgets.Row{Art: &spec, Title: e.Title, Subtitle: sub, Playing: e.Path != "" && e.Path == playing}
	switch {
	case e.State == audiobooks.Failed:
		r.Subtitle = e.Err
	case e.State == audiobooks.Queued:
		r.Duration = "…"
	case e.State == audiobooks.Downloading:
		r.Duration = strconv.Itoa(int(e.Progress*100)) + "%"
	case e.State == audiobooks.Decrypting:
		r.Duration = "…"
	case e.Path != "":
		r.Subtitle += " · On this card"
	case e.Finished:
		r.Subtitle += " · Finished"
	case e.Percent >= 1:
		r.Subtitle += " · " + strconv.Itoa(int(e.Percent)) + "% listened"
	}
	return r
}

// Draw renders the frame.
func (b *Books) Draw(u *ui.Context) {
	b.sync()
	title, body, detail, empty := b.emptyState()
	if empty {
		widgets.DrawBackground(u, widgets.StateGround)
		widgets.EmptyState(u, &b.state, title, body, detail)
	} else {
		widgets.DrawBackground(u, widgets.TabGround)
		playing := b.p.State().Track.Path
		u.PushClip(screens.GlowArea)
		b.view.Draw(u.Now, side, contentTop, func(i int, x, y, k float32) {
			box := gfx.Rect{X: x, Y: y, W: b.view.CellW, H: widgets.RowHeight}
			widgets.RowGlow(u, box, widgets.RowRadius, k)
			u.PushClip(screens.Below(contentTop))
			widgets.ListRow(u, rowOf(b.entries[i], playing), box, k)
			u.PopClip()
		})
		u.PopClip()
	}
	widgets.StatusBar(u)
	widgets.Tabs(u, BooksTab)
	widgets.HintBar(u, b.hints())
}

// emptyState says what to show in place of the list, if the list can't be.
func (b *Books) emptyState() (title, body, detail string, ok bool) {
	if !b.cat.SignedIn() {
		return "Audible isn't set up", "Copy your Audible registration, the file auth.json, to this folder to see your books here.", b.setup, true
	}
	if len(b.entries) > 0 {
		return "", "", "", false
	}
	switch st, msg := b.cat.Status(); st {
	case audiobooks.Offline:
		return "No network", "Connect to Wi-Fi to read your Audible library.", "", true
	case audiobooks.Broken:
		return "Couldn't read your library", msg, "", true
	case audiobooks.OK:
		return "Your Audible library is empty", "Books you add to it show up here.", "", true
	}
	return "Reading your Audible library", "One moment.", "", true
}

func (b *Books) hints() []widgets.Hint {
	if !b.cat.SignedIn() || len(b.entries) == 0 {
		return []widgets.Hint{{Key: "X", Label: "Refresh"}, {Key: "L1/R1", Label: "Tabs"}}
	}
	label, queue := "Download", false
	switch e := b.entries[min(b.view.Focus(), len(b.entries)-1)]; {
	case e.Path != "":
		label, queue = "Play", true
	case e.State == audiobooks.Queued || e.State == audiobooks.Downloading || e.State == audiobooks.Decrypting:
		label = "On its way"
	case e.State == audiobooks.Failed:
		label = "Try again"
	}
	hs := []widgets.Hint{{Key: "A", Label: label}}
	if queue {
		hs = append(hs, widgets.Hint{Key: "Y", Label: "Add to queue"})
	}
	return append(hs, widgets.Hint{Key: "X", Label: "Refresh"}, widgets.Hint{Key: "L1/R1", Label: "Tabs"})
}
