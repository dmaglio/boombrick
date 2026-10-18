package spotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// Client is what the Library asks the helper. *Helper is one.
type Client interface {
	Status(ctx context.Context) (Status, error)
	Login(ctx context.Context) (Login, error)
	Logout(ctx context.Context) error
	Playlists(ctx context.Context) ([]Playlist, error)
	Tracks(ctx context.Context, uri string) ([]Track, error)
}

const (
	// fresh is how long a list is shown before the helper is asked again.
	fresh = 10 * time.Minute
	// retryAfter is how long a list whose asking failed waits before it is
	// asked again, unless Retry says to at once.
	retryAfter = 30 * time.Second
	// statusEvery is how often the status is asked while a screen shows it.
	statusEvery = time.Second
	// playlistsKey is the playlists' key in the cache; a playlist's tracks
	// are under its URI.
	playlistsKey = "playlists"
)

// Library is the Spotify library as the screens ask it: each call returns
// what is known at once and asks the helper for what isn't, on goroutines of
// its own. Lists are kept in the store's spotify_cache table, so they show at
// once after a restart, and are asked for again after ten minutes.
type Library struct {
	c     Client
	db    *sql.DB
	log   *slog.Logger
	now   func() time.Time
	ready chan struct{}
	ctx   context.Context
	stop  context.CancelFunc
	wg    sync.WaitGroup

	mu        sync.Mutex
	status    Status
	statusErr error
	statusAt  time.Time
	loginErr  error // why the last sign-in failed, until the next
	asking    bool  // a status request is under way
	lists     map[string]*list
}

// list is one list as known: what was had and when, and how asking for it
// last went.
type list struct {
	body  any // []Playlist or []Track
	have  bool
	at    time.Time // when body was fetched
	err   error     // the last asking's failure, cleared by a success
	tried time.Time // when the last asking ended
	busy  bool      // the store or the helper is being asked
	read  bool      // the store's copy has been read
}

// NewLibrary asks c, keeping lists in db.
func NewLibrary(c Client, db *sql.DB, log *slog.Logger) *Library {
	ctx, stop := context.WithCancel(context.Background())
	return &Library{c: c, db: db, log: log, now: time.Now, ready: make(chan struct{}, 1), ctx: ctx, stop: stop,
		lists: map[string]*list{}}
}

// Ready receives a value when an answer is in.
func (l *Library) Ready() <-chan struct{} { return l.ready }

// Close stops the asking under way and waits for it.
func (l *Library) Close() {
	l.stop()
	l.wg.Wait()
}

func (l *Library) signal() {
	select {
	case l.ready <- struct{}{}:
	default:
	}
}

// Status is the last status the helper gave, with the error asking it got
// if it couldn't be asked; it is asked again once it is a second old. The
// State is "" until the first answer.
func (l *Library) Status() (Status, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.asking && l.now().Sub(l.statusAt) >= statusEvery {
		l.asking = true
		l.wg.Add(1)
		go l.askStatus()
	}
	return l.status, l.statusErr
}

func (l *Library) askStatus() {
	defer l.wg.Done()
	ctx, cancel := context.WithTimeout(l.ctx, 5*time.Second)
	defer cancel()
	st, err := l.c.Status(ctx)
	l.mu.Lock()
	l.asking, l.statusAt = false, l.now()
	changed := err != l.statusErr || st.State != l.status.State || st.Name != l.status.Name || st.Premium != l.status.Premium ||
		st.Login != nil && (l.status.Login == nil || *st.Login != *l.status.Login)
	l.statusErr = err
	other := err == nil && st.User != "" && l.status.User != "" && st.User != l.status.User
	if err == nil {
		l.status = st
	}
	l.mu.Unlock()
	if other {
		l.forget() // signed in as someone else
	}
	if changed || other {
		l.signal()
	}
}

// Playlists are the user's playlists; have is false until they are known,
// and err the last asking's failure, even when what was had before shows.
func (l *Library) Playlists() (ps []Playlist, have bool, err error) {
	v, have, err := l.get(playlistsKey, func(ctx context.Context) (any, error) { return l.c.Playlists(ctx) })
	ps, _ = v.([]Playlist)
	return ps, have, err
}

// Tracks are a playlist's tracks, or Liked Songs' (LikedURI), as Playlists.
func (l *Library) Tracks(uri string) (ts []Track, have bool, err error) {
	v, have, err := l.get(uri, func(ctx context.Context) (any, error) { return l.c.Tracks(ctx, uri) })
	ts, _ = v.([]Track)
	return ts, have, err
}

// Saved is the playlists as last kept, without asking the helper, so Home
// can show them without starting it.
func (l *Library) Saved() ([]Playlist, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entry(playlistsKey)
	if !e.read && !e.busy {
		e.busy = true
		l.wg.Add(1)
		go l.fill(playlistsKey, e, nil)
	}
	ps, _ := e.body.([]Playlist)
	return ps, e.have
}

// Retry asks again at once for what failed.
func (l *Library) Retry() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.lists {
		if e.err != nil {
			e.tried = time.Time{}
		}
	}
	l.statusAt = time.Time{}
}

// Login starts a sign-in; its code comes in the Status.
func (l *Library) Login() {
	l.mu.Lock()
	l.loginErr = nil
	l.mu.Unlock()
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		ctx, cancel := context.WithTimeout(l.ctx, 20*time.Second)
		defer cancel()
		_, err := l.c.Login(ctx)
		l.mu.Lock()
		l.statusAt = time.Time{} // ask again at once
		l.loginErr = err
		l.mu.Unlock()
		l.signal()
	}()
}

// LoginErr is why the last sign-in failed, until the next one starts. It is
// kept apart from the status's error: the helper answers the status after a
// sign-in fails, signed out, and that would hide why.
func (l *Library) LoginErr() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loginErr
}

// Logout signs out, and forgets the lists.
func (l *Library) Logout() {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		ctx, cancel := context.WithTimeout(l.ctx, 10*time.Second)
		defer cancel()
		if err := l.c.Logout(ctx); err != nil {
			l.log.Warn("spotify sign out", "err", err)
		}
		l.forget()
		l.mu.Lock()
		l.statusAt = time.Time{}
		l.mu.Unlock()
		l.signal()
	}()
}

// forget drops the lists, kept and in the store.
func (l *Library) forget() {
	l.mu.Lock()
	l.lists = map[string]*list{}
	l.mu.Unlock()
	if _, err := l.db.Exec(`DELETE FROM spotify_cache`); err != nil {
		l.log.Warn("forget spotify lists", "err", err)
	}
}

func (l *Library) entry(key string) *list {
	e := l.lists[key]
	if e == nil {
		e = &list{}
		l.lists[key] = e
	}
	return e
}

// get returns the list under key as known, and asks for it when it is due:
// first from the store, then from the helper with fetch when the store has
// none or it is old.
func (l *Library) get(key string, fetch func(context.Context) (any, error)) (any, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entry(key)
	now := l.now()
	due := !e.read || !e.have || now.Sub(e.at) >= fresh
	if e.err != nil && now.Sub(e.tried) < retryAfter {
		due = false
	}
	if due && !e.busy {
		e.busy = true
		l.wg.Add(1)
		go l.fill(key, e, fetch)
	}
	return e.body, e.have, e.err
}

// fill reads e from the store the first time, then asks the helper with
// fetch unless the store's copy is fresh or there is no fetch.
func (l *Library) fill(key string, e *list, fetch func(context.Context) (any, error)) {
	defer l.wg.Done()
	l.mu.Lock()
	read := e.read
	l.mu.Unlock()
	if !read {
		body, at, ok := l.load(key)
		l.mu.Lock()
		e.read = true
		if ok && !e.have {
			e.body, e.have, e.at = body, true, at
		}
		done := fetch == nil || e.have && l.now().Sub(e.at) < fresh
		if done {
			e.busy = false
		}
		l.mu.Unlock()
		if ok {
			l.signal()
		}
		if done {
			return
		}
	}
	ctx, cancel := context.WithTimeout(l.ctx, time.Minute)
	defer cancel()
	v, err := fetch(ctx)
	if err == nil {
		l.save(key, v)
	}
	l.mu.Lock()
	e.busy, e.tried = false, l.now()
	if err != nil {
		e.err = err
	} else {
		e.body, e.have, e.at, e.err = v, true, l.now(), nil
	}
	l.mu.Unlock()
	if l.ctx.Err() == nil {
		l.signal()
	}
}

// load reads key's list from the store.
func (l *Library) load(key string) (any, time.Time, bool) {
	var body string
	var at int64
	err := l.db.QueryRow(`SELECT body, fetched FROM spotify_cache WHERE key = ?`, key).Scan(&body, &at)
	if err != nil {
		if err != sql.ErrNoRows {
			l.log.Warn("read spotify list", "key", key, "err", err)
		}
		return nil, time.Time{}, false
	}
	var v any
	if key == playlistsKey {
		var ps []Playlist
		err = json.Unmarshal([]byte(body), &ps)
		v = ps
	} else {
		var ts []Track
		err = json.Unmarshal([]byte(body), &ts)
		v = ts
	}
	if err != nil {
		l.log.Warn("read spotify list", "key", key, "err", err)
		return nil, time.Time{}, false
	}
	return v, time.UnixMilli(at), true
}

// save keeps key's list in the store.
func (l *Library) save(key string, v any) {
	body, err := json.Marshal(v)
	if err == nil {
		_, err = l.db.Exec(`INSERT INTO spotify_cache (key, body, fetched) VALUES (?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET body = excluded.body, fetched = excluded.fetched`,
			key, string(body), l.now().UnixMilli())
	}
	if err != nil {
		l.log.Warn("keep spotify list", "key", key, "err", err)
	}
}
