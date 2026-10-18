package download

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/lyrics"
	"github.com/ujjwal313/boombrick/internal/remux"
	"github.com/ujjwal313/boombrick/internal/ytmusic"
)

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		`AC/DC: Live?`:       "AC_DC_ Live_",
		"  Night Drive.  ":   "Night Drive",
		"...":                "Unknown",
		".hack":              "hack",
		"tab\there":          "tab_here",
		`"Quoted" <a|b> \c*`: "_Quoted_ _a_b_ _c_",
	}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
	long := safeName(strings.Repeat("夜", 100)) // 300 bytes
	if len(long) > maxName || !utf8.ValidString(long) {
		t.Fatalf("a long name: %d bytes, valid %v", len(long), utf8.ValidString(long))
	}
}

func TestPlace(t *testing.T) {
	music := t.TempDir()
	s := ytmusic.Song{Title: "Exit 41", AlbumArtist: "Lumen Coast", Album: "Night Drive Theory", Number: 5}
	want := filepath.Join(music, "Boombrick", "Lumen Coast", "Night Drive Theory", "05 Exit 41.opus")
	if got := place(music, s); got != want {
		t.Fatalf("place = %q", got)
	}
	os.MkdirAll(filepath.Dir(want), 0o755)
	os.WriteFile(want, nil, 0o644)
	if got := place(music, s); got != strings.TrimSuffix(want, ".opus")+" (2).opus" {
		t.Fatalf("a taken name: %q", got)
	}
	s.Number = 0
	if got := filepath.Base(place(music, s)); got != "Exit 41.opus" {
		t.Fatalf("without a number: %q", got)
	}
}

// A song filed just before a power cut can lose its row and be downloaded
// again. Its file, tagged with the song's YouTube ID, is replaced rather
// than kept beside a "(2)".
func TestPlaceReplacesTheSameSong(t *testing.T) {
	src := filepath.Join("..", "remux", "testdata", "tone.webm")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("%s is missing; run make fixtures", src)
	}
	music := t.TempDir()
	s := ytmusic.Song{VideoID: "v1", Title: "Exit 41", AlbumArtist: "Lumen Coast", Album: "Night Drive Theory", Number: 5}
	first := place(music, s)
	os.MkdirAll(filepath.Dir(first), 0o755)
	if err := remux.File(src, first, remux.Tags{Title: s.Title, Extra: [][2]string{{"BOOMBRICK_YOUTUBE_ID", "v1"}}}); err != nil {
		t.Fatal(err)
	}
	if got := place(music, s); got != first {
		t.Fatalf("its own file: %q", got)
	}
	s.VideoID = "v2" // another song of the same name
	if got := place(music, s); got != strings.TrimSuffix(first, ".opus")+" (2).opus" {
		t.Fatalf("another song's file: %q", got)
	}
}

type fakeAlbums struct {
	page ytmusic.AlbumPage
	err  error
}

func (f fakeAlbums) Album(ctx context.Context, id string) (ytmusic.AlbumPage, error) {
	if id != f.page.ID {
		return ytmusic.AlbumPage{}, errors.New("no such album")
	}
	return f.page, f.err
}

var nightDrivePage = ytmusic.AlbumPage{ID: "MPREb_nd", Title: "Night Drive Theory", Artist: "Lumen Coast", Year: 2023,
	Thumb: "https://example.com/nd-544", Tracks: []ytmusic.Track{
		{VideoID: "v1", Number: 1, Title: "Night Drive Theory"},
		{VideoID: "v3", Number: 3, Title: "Overpass", Artist: "Lumen Coast & Vela"},
	}}

func TestComplete(t *testing.T) {
	f := &finisher{albums: fakeAlbums{page: nightDrivePage}, log: slog.New(slog.DiscardHandler)}
	s := f.complete(context.Background(), ytmusic.Song{VideoID: "v3", Title: "Overpass", Artist: "Lumen Coast & Vela",
		Album: "Night Drive Theory", AlbumID: "MPREb_nd", Thumb: "https://example.com/small"})
	if s.Number != 3 || s.Year != 2023 || s.AlbumArtist != "Lumen Coast" || s.Thumb != "https://example.com/nd-544" {
		t.Fatalf("completed %+v", s)
	}
	single := f.complete(context.Background(), ytmusic.Song{VideoID: "s1", Title: "Harbour Static", Artist: "Vela"})
	if single.Album != "Harbour Static" || single.AlbumArtist != "Vela" {
		t.Fatalf("a song without an album: %+v", single)
	}
	f.albums = fakeAlbums{page: nightDrivePage, err: errors.New("offline")}
	if s := f.complete(context.Background(), ytmusic.Song{VideoID: "v1", Title: "Night Drive Theory", Artist: "Lumen Coast",
		Album: "Night Drive Theory", AlbumID: "MPREb_nd"}); s.Number != 0 || s.AlbumArtist != "Lumen Coast" {
		t.Fatalf("a failed lookup: %+v", s)
	}
}

type fakeLyrics struct{ synced string }

func (f fakeLyrics) Get(ctx context.Context, t lyrics.Track) (lyrics.Result, error) {
	if t.Title != "Night Drive Theory" || t.Duration != 231*time.Second {
		return lyrics.Result{}, lyrics.ErrNotFound
	}
	return lyrics.Result{Synced: f.synced, Plain: "plain"}, nil
}

func TestFinish(t *testing.T) {
	// A 600 x 400 PNG: not square, so it is cropped, sized and made a JPEG.
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 600, 400)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(pic.Bytes()) }))
	defer srv.Close()

	db, music, parts := openDB(t), t.TempDir(), t.TempDir()
	src := filepath.Join(parts, "v1.webm")
	os.WriteFile(src, []byte("webm"), 0o644)
	var tags remux.Tags
	f := &finisher{db: db, music: music, http: srv.Client(), lyrics: fakeLyrics{synced: "[00:01.00]Headlights"},
		formats: library.Formats{Supported: func(string) bool { return true }, Duration: func(string) (time.Duration, error) { return 0, nil }},
		remux: func(src, dst string, t remux.Tags) error {
			tags = t
			return os.WriteFile(dst, []byte("opus from "+src), 0o644)
		}, log: slog.New(slog.DiscardHandler)}
	song := ytmusic.Song{VideoID: "v1", Title: "Night Drive Theory", Artist: "Lumen Coast", Album: "Night Drive Theory",
		AlbumArtist: "Lumen Coast", Number: 1, Year: 2023, Duration: 231 * time.Second, Thumb: srv.URL + "/thumb"}
	path, size, replaced, err := f.finish(context.Background(), song, src)
	if err != nil || replaced {
		t.Fatalf("finish: %v; replaced %v", err, replaced)
	}
	if want := filepath.Join(music, "Boombrick", "Lumen Coast", "Night Drive Theory", "01 Night Drive Theory.opus"); path != want {
		t.Fatalf("filed at %q", path)
	}
	if size != int64(len("opus from "+src)) {
		t.Fatalf("size %d", size)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("the WebM was kept")
	}
	if tags.Title != "Night Drive Theory" || tags.Track != 1 || tags.Year != 2023 || tags.Lyrics != "[00:01.00]Headlights" ||
		len(tags.Extra) != 1 || tags.Extra[0] != [2]string{"BOOMBRICK_YOUTUBE_ID", "v1"} {
		t.Fatalf("tags %+v", tags)
	}
	if p := tags.Picture; p == nil || p.MIME != "image/jpeg" || p.Width != 400 || p.Height != 400 {
		t.Fatalf("cover %+v", p)
	}
	albums, err := library.Albums(db)
	if err != nil || len(albums) != 1 || albums[0].Tracks[0].Path != path {
		t.Fatalf("library %+v, %v", albums, err)
	}

	// A failed remux fails it, and keeps the WebM for another try.
	os.WriteFile(src, []byte("webm"), 0o644)
	f.remux = func(string, string, remux.Tags) error { return remux.ErrNotOpus }
	if _, _, _, err := f.finish(context.Background(), song, src); !errors.Is(err, remux.ErrNotOpus) {
		t.Fatalf("a failed remux: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("the WebM went with a failed remux")
	}
}

func TestSafeNameWindows(t *testing.T) {
	cases := map[string]string{
		"con":      "con_",
		"CON.txt":  "CON_.txt",
		"COM1":     "COM1_",
		"lpt9":     "lpt9_",
		"Nul ":     "Nul_",
		"COM10":    "COM10",
		"Console":  "Console",
		". hack":   "hack",
		"  .. a  ": "a",
		"":         "Unknown",
		".":        "Unknown",
		"..":       "Unknown",
	}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlaceOddTitlesAndThirdCopy(t *testing.T) {
	music := t.TempDir()
	for _, title := range []string{"", ".", ".."} {
		s := ytmusic.Song{Title: title, AlbumArtist: "A", Album: "B"}
		if got := filepath.Base(place(music, s)); got != "Unknown.opus" {
			t.Errorf("title %q: %q", title, got)
		}
	}
	s := ytmusic.Song{Title: "T", AlbumArtist: "A", Album: "B"}
	dir := filepath.Join(music, "Boombrick", "A", "B")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "T.opus"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "T (2).opus"), nil, 0o644)
	if got := filepath.Base(place(music, s)); got != "T (3).opus" {
		t.Fatalf("a third copy: %q", got)
	}
}

func TestCompleteKeepsAlbumDetailsWhenSongIsNotListed(t *testing.T) {
	f := &finisher{albums: fakeAlbums{page: nightDrivePage}, log: slog.New(slog.DiscardHandler)}
	s := f.complete(context.Background(), ytmusic.Song{VideoID: "other", Title: "Bonus", Artist: "Guest",
		Album: "Night Drive Theory", AlbumID: "MPREb_nd", Thumb: "https://example.com/small"})
	if s.AlbumArtist != "Lumen Coast" || s.Thumb != "https://example.com/nd-544" || s.Year != 2023 || s.Number != 0 {
		t.Fatalf("completed %+v", s)
	}
}

// testFinisher is a finisher over a fresh library whose remux writes a stub
// and records the tags.
func testFinisher(t *testing.T, srv *httptest.Server, tags *remux.Tags) (*finisher, string, string) {
	t.Helper()
	music, parts := t.TempDir(), t.TempDir()
	src := filepath.Join(parts, "v1.webm")
	os.WriteFile(src, []byte("webm"), 0o644)
	f := &finisher{db: openDB(t), music: music, http: srv.Client(),
		formats: library.Formats{Supported: func(string) bool { return true }, Duration: func(string) (time.Duration, error) { return 0, nil }},
		remux: func(src, dst string, t remux.Tags) error {
			*tags = t
			return os.WriteFile(dst, []byte("opus"), 0o644)
		}, log: slog.New(slog.DiscardHandler)}
	return f, music, src
}

var fixSong = ytmusic.Song{VideoID: "v1", Title: "Night Drive Theory", Artist: "Lumen Coast", Album: "Night Drive Theory",
	AlbumArtist: "Lumen Coast", Number: 1, Duration: 231 * time.Second}

func TestFinishCancelled(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	var tags remux.Tags
	f, music, src := testFinisher(t, srv, &tags)
	f.remux = func(string, string, remux.Tags) error { t.Error("remuxed after a cancel"); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := f.finish(ctx, fixSong, src); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("the WebM went with a cancel")
	}
	if _, err := os.Stat(filepath.Join(music, "Boombrick")); err != nil {
		t.Fatal(err) // only the Boombrick folder is left
	}
	if m, _ := filepath.Glob(filepath.Join(music, "Boombrick", "*")); len(m) != 0 {
		t.Fatalf("folders left behind: %v", m)
	}
}

func TestFinishFailedRemuxTidiesFolders(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	var tags remux.Tags
	f, music, src := testFinisher(t, srv, &tags)
	f.remux = func(string, string, remux.Tags) error { return remux.ErrNotOpus }
	if _, _, _, err := f.finish(context.Background(), fixSong, src); !errors.Is(err, remux.ErrNotOpus) {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(music, "Boombrick", "*")); len(m) != 0 {
		t.Fatalf("folders left behind: %v", m)
	}
}

func TestFinishSurvivesLookupFailures(t *testing.T) {
	// A 404 and an undecodable body each leave the cover out.
	for name, h := range map[string]http.HandlerFunc{
		"404":  http.NotFound,
		"junk": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("not an image")) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			var tags remux.Tags
			f, _, src := testFinisher(t, srv, &tags)
			f.lyrics = failingLyrics{}
			s := fixSong
			s.Thumb = srv.URL + "/thumb"
			if _, _, _, err := f.finish(context.Background(), s, src); err != nil {
				t.Fatal(err)
			}
			if tags.Picture != nil || tags.Lyrics != "" || tags.Title != s.Title {
				t.Fatalf("tags %+v", tags)
			}
		})
	}
}

type failingLyrics struct{}

func (failingLyrics) Get(context.Context, lyrics.Track) (lyrics.Result, error) {
	return lyrics.Result{}, errors.New("offline")
}

func TestFinishPlainLyrics(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	var tags remux.Tags
	f, _, src := testFinisher(t, srv, &tags)
	f.lyrics = fakeLyrics{}
	if _, _, _, err := f.finish(context.Background(), fixSong, src); err != nil {
		t.Fatal(err)
	}
	if tags.Lyrics != "plain" {
		t.Fatalf("lyrics %q", tags.Lyrics)
	}
}

func jpegOf(px int) []byte {
	var b bytes.Buffer
	jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, px, px)), nil)
	return b.Bytes()
}

func TestCoverSizes(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	f := &finisher{http: srv.Client(), log: slog.New(slog.DiscardHandler)}

	body = jpegOf(coverPx)
	if p := f.cover(context.Background(), srv.URL+"/a"); p == nil || !bytes.Equal(p.Data, body) || p.Width != coverPx {
		t.Fatalf("a square at the size is not passed through: %+v", p)
	}
	body = jpegOf(800)
	p := f.cover(context.Background(), srv.URL+"/b")
	if p == nil || p.Width != coverPx || p.Height != coverPx || bytes.Equal(p.Data, body) {
		t.Fatalf("a large square is not sized down: %+v", p)
	}
}

// A song an import matched carries its Spotify ID and ISRC into the file.
func TestFinishTagsImportedSongs(t *testing.T) {
	db, music, parts := openDB(t), t.TempDir(), t.TempDir()
	src := filepath.Join(parts, "v1.webm")
	os.WriteFile(src, []byte("webm"), 0o644)
	var tags remux.Tags
	f := &finisher{db: db, music: music, http: http.DefaultClient, lyrics: fakeLyrics{},
		formats: library.Formats{Supported: func(string) bool { return true }, Duration: func(string) (time.Duration, error) { return 0, nil }},
		remux: func(src, dst string, t remux.Tags) error {
			tags = t
			return os.WriteFile(dst, []byte("opus"), 0o644)
		}, log: slog.New(slog.DiscardHandler)}
	song := ytmusic.Song{VideoID: "v1", Title: "Sapphire", Artist: "Ed Sheeran", Album: "Sapphire",
		SpotifyID: "4Q0qVhFQa7j6jRKzo3HDmP", ISRC: "GBAHS2500262"}
	if _, _, _, err := f.finish(context.Background(), song, src); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{{"BOOMBRICK_YOUTUBE_ID", "v1"}, {"BOOMBRICK_SPOTIFY_ID", "4Q0qVhFQa7j6jRKzo3HDmP"}, {"ISRC", "GBAHS2500262"}}
	if !slices.Equal(tags.Extra, want) {
		t.Fatalf("tags %v", tags.Extra)
	}
}
