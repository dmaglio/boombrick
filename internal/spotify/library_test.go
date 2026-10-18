package spotify

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ujjwal313/boombrick/internal/store"
)

// fakeClient answers from its fields and counts what it is asked.
type fakeClient struct {
	mu        sync.Mutex
	status    Status
	playlists []Playlist
	tracks    map[string][]Track
	err       error // every list's answer, when set
	asked     map[string]int
}

func (f *fakeClient) count(what string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked[what]++
}

func (f *fakeClient) times(what string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked[what]
}

func (f *fakeClient) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *fakeClient) Status(context.Context) (Status, error) {
	f.count("status")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}

func (f *fakeClient) Login(context.Context) (Login, error) {
	f.count("login")
	f.set(func() {
		f.status = Status{State: SigningIn, Login: &Login{URL: "https://spotify.com/pair?code=ABCD", Code: "ABCD"}}
	})
	return Login{}, nil
}

func (f *fakeClient) Logout(context.Context) error {
	f.count("logout")
	f.set(func() { f.status = Status{State: SignedOut} })
	return nil
}

func (f *fakeClient) Playlists(context.Context) ([]Playlist, error) {
	f.count(playlistsKey)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playlists, f.err
}

func (f *fakeClient) Tracks(_ context.Context, uri string) ([]Track, error) {
	f.count(uri)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tracks[uri], f.err
}

var (
	gym   = []Playlist{{URI: "spotify:playlist:5ABHKGoOzxkaa28ttQV9sE", Name: "Gym", Owner: "Ujjwal", Count: 1}}
	songs = []Track{{ID: "3UmaczJpikHgJFyBTAJVoz", Title: "Stan", Artists: []string{"Eminem", "Dido"}, Duration: 404000}}
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "boombrick.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s.DB
}

// clock is a time the test moves on.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) time() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// library makes a Library on db with a clock the test moves.
func library(t *testing.T, c Client, db *sql.DB) (*Library, *clock) {
	t.Helper()
	l := NewLibrary(c, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clk := &clock{now: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	l.now = clk.time
	t.Cleanup(l.Close)
	return l, clk
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); !cond(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// Lists come from the helper the first time, then from the store, without
// asking the helper again while they are fresh.
func TestListsAreKept(t *testing.T) {
	db := openDB(t)
	c := &fakeClient{playlists: gym, tracks: map[string][]Track{LikedURI: songs}, asked: map[string]int{}}
	l, _ := library(t, c, db)
	if _, have, _ := l.Playlists(); have {
		t.Fatal("playlists known before anything was asked")
	}
	waitFor(t, "the playlists", func() bool { _, have, _ := l.Playlists(); return have })
	waitFor(t, "Liked Songs", func() bool { _, have, _ := l.Tracks(LikedURI); return have })
	if ps, _, _ := l.Playlists(); !reflect.DeepEqual(ps, gym) {
		t.Fatalf("playlists %+v", ps)
	}

	c2 := &fakeClient{err: ErrOffline, asked: map[string]int{}}
	l2, _ := library(t, c2, db)
	waitFor(t, "the kept playlists", func() bool { _, have, _ := l2.Playlists(); return have })
	waitFor(t, "the kept tracks", func() bool { _, have, _ := l2.Tracks(LikedURI); return have })
	if ts, _, err := l2.Tracks(LikedURI); !reflect.DeepEqual(ts, songs) || err != nil {
		t.Fatalf("tracks %+v, %v", ts, err)
	}
	time.Sleep(20 * time.Millisecond)
	if n := c2.times(playlistsKey) + c2.times(LikedURI); n != 0 {
		t.Fatalf("asked the helper %d times for fresh lists", n)
	}
}

// After ten minutes a list is asked for again; one that can't be had keeps
// what was had, with the error, and isn't asked again for a while unless
// Retry says so.
func TestOldListsAreAskedAgain(t *testing.T) {
	c := &fakeClient{playlists: gym, asked: map[string]int{}}
	l, now := library(t, c, openDB(t))
	waitFor(t, "the playlists", func() bool { _, have, _ := l.Playlists(); return have })
	c.set(func() { c.err = ErrOffline })
	now.advance(11 * time.Minute)
	waitFor(t, "the second asking", func() bool { _, _, err := l.Playlists(); return err == ErrOffline })
	if ps, have, _ := l.Playlists(); !have || !reflect.DeepEqual(ps, gym) {
		t.Fatalf("offline, the playlists are %+v, %v", ps, have)
	}
	for range 10 {
		l.Playlists()
	}
	time.Sleep(20 * time.Millisecond)
	if n := c.times(playlistsKey); n != 2 {
		t.Fatalf("asked %d times, failing", n)
	}
	c.set(func() { c.err = nil })
	l.Retry()
	waitFor(t, "the retry", func() bool { _, _, err := l.Playlists(); return err == nil })
}

// Saved shows what is kept without asking the helper.
func TestSavedDoesntAskTheHelper(t *testing.T) {
	db := openDB(t)
	c := &fakeClient{playlists: gym, asked: map[string]int{}}
	l, _ := library(t, c, db)
	waitFor(t, "the playlists", func() bool { _, have, _ := l.Playlists(); return have })

	c2 := &fakeClient{asked: map[string]int{}}
	l2, _ := library(t, c2, db)
	waitFor(t, "the kept playlists", func() bool { _, have := l2.Saved(); return have })
	if ps, _ := l2.Saved(); !reflect.DeepEqual(ps, gym) {
		t.Fatalf("saved %+v", ps)
	}
	if n := c2.times(playlistsKey) + c2.times("status"); n != 0 {
		t.Fatalf("asked the helper %d times", n)
	}
}

// The status is asked once a second; signing in and out ask it again at
// once, and signing out forgets the lists.
func TestStatusSignInAndOut(t *testing.T) {
	db := openDB(t)
	c := &fakeClient{status: Status{State: SignedOut}, playlists: gym, asked: map[string]int{}}
	l, now := library(t, c, db)
	waitFor(t, "the status", func() bool { st, _ := l.Status(); return st.State == SignedOut })
	for range 10 {
		l.Status()
	}
	if n := c.times("status"); n != 1 {
		t.Fatalf("asked the status %d times within a second", n)
	}
	l.Login()
	waitFor(t, "the code", func() bool { st, _ := l.Status(); return st.State == SigningIn && st.Login != nil })

	c.set(func() {
		c.status = Status{State: Ready, User: "bmfjclzv7pstc3zzlq0nteyni", Name: "Ujjwal", Premium: true}
	})
	now.advance(statusEvery)
	waitFor(t, "signed in", func() bool { st, _ := l.Status(); return st.State == Ready && st.Name == "Ujjwal" })
	waitFor(t, "the playlists", func() bool { _, have, _ := l.Playlists(); return have })

	l.Logout()
	waitFor(t, "signed out", func() bool { st, _ := l.Status(); return st.State == SignedOut })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM spotify_cache`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d lists kept after signing out, %v", n, err)
	}
	if _, have := l.Saved(); have {
		t.Fatal("the playlists are still known after signing out")
	}
}
