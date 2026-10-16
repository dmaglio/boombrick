package browse

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/ujjwal313/boombrick/internal/fixture"
	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/uitest"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

func TestMain(m *testing.M) { uitest.Main(m) }

var t0 = time.Unix(1000, 0)

func press(b input.Button) input.Event { return input.Event{Button: b, Kind: input.Press} }

// fakePlayer records what the screens ask of the player.
type fakePlayer struct {
	played   []string // first path of each Play
	start    []int
	queued   [][]string
	shuffled []bool
	playing  string
}

func (p *fakePlayer) State() player.State {
	return player.State{Queued: p.playing != "", Track: player.Track{Path: p.playing}}
}
func (p *fakePlayer) Play(ts []player.Track, start int) {
	p.played, p.start = append(p.played, ts[0].Path), append(p.start, start)
}
func (p *fakePlayer) SetShuffle(on bool) { p.shuffled = append(p.shuffled, on) }
func (p *fakePlayer) Enqueue(ts []player.Track) {
	var paths []string
	for _, t := range ts {
		paths = append(paths, t.Path)
	}
	p.queued = append(p.queued, paths)
}

// fakeNav records pushes and toasts.
type fakeNav struct {
	pushed []screens.Screen
	toasts []string
}

func (n *fakeNav) Push(s screens.Screen) { n.pushed = append(n.pushed, s) }
func (n *fakeNav) Pop()                  {}
func (n *fakeNav) Toast(msg string)      { n.toasts = append(n.toasts, msg) }

// sampleIndex is a small library with names from the design.
func sampleIndex() *library.Index {
	track := func(album, title string, n int, d time.Duration) library.Track {
		return library.Track{Path: "/m/" + album + "/" + title + ".flac", Title: title, Artist: "Odalys", Album: album,
			Number: n, Duration: d}
	}
	albums := []library.Album{
		{Artist: "Odalys", Title: "Copper Season", Year: 2019, Duration: 15 * time.Minute, Tracks: []library.Track{
			track("Copper Season", "Rust", 1, 222*time.Second), track("Copper Season", "Ember", 2, 245*time.Second),
			track("Copper Season", "Tin Roof", 3, 178*time.Second), track("Copper Season", "Low Tide", 4, 210*time.Second),
		}},
		{Artist: "Kenji Arata", Title: "Afterhours", Year: 2021, Duration: 4 * time.Minute, Tracks: []library.Track{
			{Path: "/m/a/1.flac", Title: "Neon", Artist: "Kenji Arata", Album: "Afterhours", Duration: 4 * time.Minute},
		}},
		{Artist: "Odalys", Title: "Northbound", Year: 2016, Tracks: []library.Track{
			{Path: "/m/n/1.flac", Title: "Salt", Artist: "Odalys", Album: "Northbound"},
		}},
	}
	return library.NewIndex(albums)
}

func render(t *testing.T, s screens.Screen, now time.Time) *image.NRGBA {
	t.Helper()
	u, off := uitest.NewContext(t, theme.DeviceW, theme.DeviceH)
	u.Accent = theme.Derive(fixture.PlayingHue, fixture.PlayingChroma)
	u.Status = fixture.Status()
	u.Now = now
	s.Draw(u)
	return off.Image()
}

// fadesSoftly fails if the frame steps sharply anywhere on the line from one
// device pixel to another, as a focus glow cut off by a clip does.
func fadesSoftly(t *testing.T, img *image.NRGBA, from, to image.Point) {
	t.Helper()
	if p, found := uitest.HardEdge(img, from, to); found {
		t.Fatalf("a hard edge at %v, between %v and %v", p, from, to)
	}
}

// The first track sits just under the tabs; its glow fades out under them.
func TestAlbumPageFirstTrackGlow(t *testing.T) {
	pg := NewAlbumPage(&fakeNav{}, &fakePlayer{}, sampleIndex().Album("Odalys", "Copper Season"))
	pg.Handle(press(input.Right), t0)
	pg.Handle(press(input.Right), t0) // the first track
	img := render(t, pg, t0.Add(anim.Focus))
	uitest.Golden(t, "album-first-track", img)
	// Between the YouTube and Downloads tabs, down to the track.
	fadesSoftly(t, img, image.Pt(700, 120), image.Pt(700, 154))
}

func TestAlbumPage(t *testing.T) {
	ix := sampleIndex()
	p, nav := &fakePlayer{playing: "/m/Copper Season/Ember.flac"}, &fakeNav{}
	pg := NewAlbumPage(nav, p, ix.Album("Odalys", "Copper Season"))
	uitest.Golden(t, "album", render(t, pg, t0))

	pg.Handle(press(input.A), t0) // Play: in order, from the top
	if !slices.Equal(p.shuffled, []bool{false}) || p.start[0] != 0 {
		t.Fatalf("Play pill: shuffle %v, start %v", p.shuffled, p.start)
	}
	pg.Handle(press(input.Right), t0) // Shuffle
	if got := pg.hints()[0]; got.Key != "A" || got.Label != "Shuffle" {
		t.Fatalf("A's hint on the Shuffle pill = %+v", got)
	}
	pg.Handle(press(input.A), t0)
	if p.shuffled[1] != true {
		t.Fatal("the Shuffle pill should turn shuffle on")
	}
	pg.Handle(press(input.Right), t0) // into the tracks
	if got := pg.hints()[0].Label; got != "Play" {
		t.Fatalf("A's hint on a track = %q", got)
	}
	pg.Handle(press(input.Down), t0)
	pg.Handle(press(input.A), t0)
	if p.start[2] != 1 {
		t.Fatalf("A on the second track started at %d", p.start[2])
	}
	if !slices.Equal(p.shuffled, []bool{false, true, false}) {
		t.Fatalf("shuffle set %v: A on a track plays the album in order from it", p.shuffled)
	}
	pg.Handle(press(input.Y), t0)
	if len(p.queued) != 1 || !slices.Equal(p.queued[0], []string{"/m/Copper Season/Ember.flac"}) || nav.toasts[0] != "Added to queue" {
		t.Fatalf("Y on a track queued %v, toasts %v", p.queued, nav.toasts)
	}
	img := render(t, pg, t0.Add(anim.Focus))
	uitest.Golden(t, "album-track-focused", img)
	// From the art across to the second track.
	fadesSoftly(t, img, image.Pt(295, 312), image.Pt(322, 312))
	pg.Handle(press(input.Left), t0) // back to the pills
	pg.Handle(press(input.Y), t0)
	if len(p.queued[1]) != 4 {
		t.Fatalf("Y on the pills should queue the album, queued %v", p.queued[1])
	}
}

func TestArtistPage(t *testing.T) {
	ix := sampleIndex()
	p, nav := &fakePlayer{}, &fakeNav{}
	ar := ix.Artists[1] // Odalys: Northbound (2016), Copper Season (2019)
	pg := NewArtistPage(nav, p, ar)
	img := render(t, pg, t0)
	uitest.Golden(t, "artist", img)
	// Right of the artist's name, down to the first card.
	fadesSoftly(t, img, image.Pt(300, 150), image.Pt(300, 220))
	pg.Handle(press(input.Right), t0)
	pg.Handle(press(input.A), t0)
	if len(nav.pushed) != 1 {
		t.Fatal("A should open the album")
	}
	if ap, ok := nav.pushed[0].(*AlbumPage); !ok || ap.album.Title != "Copper Season" {
		t.Fatalf("opened %+v", nav.pushed[0])
	}
}

func TestPageHintsFit(t *testing.T) {
	u, _ := uitest.NewContext(t, 64, 64)
	ix := sampleIndex()
	shuffle := NewAlbumPage(&fakeNav{}, &fakePlayer{}, ix.Recent[0])
	shuffle.Handle(press(input.Right), t0)
	for _, hints := range [][]widgets.Hint{
		NewAlbumPage(&fakeNav{}, &fakePlayer{}, ix.Recent[0]).hints(),
		shuffle.hints(),
		NewArtistPage(&fakeNav{}, &fakePlayer{}, ix.Artists[0]).hints(),
	} {
		if w := widgets.HintsWidth(u, hints); w > widgets.MaxHintsWidth {
			t.Errorf("hints %v are %.1f wide, more than %.1f", hints, w, widgets.MaxHintsWidth)
		}
	}
}

func TestLibraryFilters(t *testing.T) {
	ix := sampleIndex()
	p, nav := &fakePlayer{}, &fakeNav{}
	l := New(nav, p, "/mnt/SDCARD/Music", nil)
	l.SetIndex(ix)
	img := render(t, l, t0)
	uitest.Golden(t, "library-albums", img)
	// Between the Albums and Artists filters, down to the first card.
	fadesSoftly(t, img, image.Pt(300, 150), image.Pt(300, 216))
	l.Handle(press(input.A), t0)
	if ap, ok := nav.pushed[0].(*AlbumPage); !ok || ap.album != ix.Albums[0] {
		t.Fatalf("A on the first album opened %+v", nav.pushed)
	}
	if !l.Handle(press(input.R2), t0) || l.filter != artistsFilter {
		t.Fatal("R2 should show artists")
	}
	img = render(t, l, t0.Add(anim.Focus))
	uitest.Golden(t, "library-artists", img)
	// Right of the filters, down to the first row.
	fadesSoftly(t, img, image.Pt(900, 150), image.Pt(900, 225))
	l.Handle(press(input.Y), t0) // Kenji Arata's one album
	if len(p.queued) != 1 || len(p.queued[0]) != 1 {
		t.Fatalf("Y on an artist queued %v", p.queued)
	}
	l.Handle(press(input.R2), t0)
	l.Handle(press(input.Down), t0)
	l.Handle(press(input.A), t0)
	if len(p.played) != 1 || p.start[0] != 1 {
		t.Fatalf("A on the second song: played %v from %v", p.played, p.start)
	}
	if !slices.Equal(p.shuffled, []bool{false}) {
		t.Fatalf("shuffle set %v: A on a song plays the list in order from it", p.shuffled)
	}
	uitest.Golden(t, "library-songs", render(t, l, t0.Add(anim.Focus)))
	if l.Handle(press(input.R2), t0) {
		t.Fatal("R2 on the last filter does nothing")
	}
	l.Handle(press(input.L2), t0)
	l.Handle(press(input.L2), t0)
	if l.filter != albumsFilter {
		t.Fatal("L2 goes back through the filters")
	}
}

func TestEmptyLibrary(t *testing.T) {
	scans := 0
	nav := &fakeNav{}
	l := New(nav, &fakePlayer{}, "/mnt/SDCARD/Music", func() { scans++ })
	if l.Handle(press(input.Y), t0) || scans != 0 {
		t.Fatal("before the index loads, Y does nothing")
	}
	l.SetIndex(library.NewIndex(nil))
	uitest.Golden(t, "library-empty", render(t, l, t0))
	if !l.Handle(press(input.Y), t0) || scans != 1 || len(nav.toasts) != 1 {
		t.Fatalf("Y on the empty library: %d scans, toasts %v", scans, nav.toasts)
	}
}

// While the first scan runs with nothing indexed, the tab says the music is
// being read, with only the tab hint, until the index arrives.
func TestLibraryWhileReading(t *testing.T) {
	scans := 0
	l := New(&fakeNav{}, &fakePlayer{}, "/mnt/SDCARD/Music", func() { scans++ })
	if got := l.hints(); len(got) != 1 || got[0].Key != "L1/R1" {
		t.Fatalf("hints before the index loads = %v, want only L1/R1 Tabs", got)
	}
	l.SetReading()
	render(t, l, t0) // the reading state; Home's golden covers how it looks
	if got := l.hints(); len(got) != 1 || got[0].Key != "L1/R1" {
		t.Fatalf("hints while reading = %v, want only L1/R1 Tabs", got)
	}
	if l.Handle(press(input.Y), t0) || scans != 0 {
		t.Fatal("Y while reading should not rescan")
	}
	l.SetIndex(sampleIndex())
	if got := l.hints(); len(got) != 4 {
		t.Fatalf("hints once the index loads = %v", got)
	}
}

func TestLibraryHintsFit(t *testing.T) {
	u, _ := uitest.NewContext(t, 64, 64)
	l := New(&fakeNav{}, &fakePlayer{}, "", nil)
	l.SetIndex(sampleIndex())
	for f := range filterCount {
		l.filter = f
		if w := widgets.HintsWidth(u, l.hints()); w > widgets.MaxHintsWidth {
			t.Errorf("filter %d hints are %.1f wide, more than %.1f", f, w, widgets.MaxHintsWidth)
		}
	}
}

func TestEmptyAlbum(t *testing.T) {
	p, nav := &fakePlayer{}, &fakeNav{}
	pg := NewAlbumPage(nav, p, &library.Album{Title: "Nothing", Artist: "Nobody"})
	pg.Handle(press(input.A), t0)
	pg.Handle(press(input.Right), t0)
	pg.Handle(press(input.A), t0) // Shuffle
	pg.Handle(press(input.Y), t0)
	if len(p.played) != 0 || len(p.queued) != 0 || len(nav.toasts) != 0 {
		t.Fatalf("an empty album played %v, queued %v, toasted %v", p.played, p.queued, nav.toasts)
	}
}

func TestSongCounts(t *testing.T) {
	if got := artistLine(1, 1); got != "1 album · 1 song" {
		t.Errorf("one album, one song: %q", got)
	}
	if got := artistLine(2, 1234); got != "2 albums · 1,234 songs" {
		t.Errorf("two albums: %q", got)
	}
}
