package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/ujjwal313/boombrick/internal/media"
	"github.com/ujjwal313/boombrick/internal/spotify"
	"github.com/ujjwal313/boombrick/internal/ytmusic"
)

// Item is one song of an import.
type Item struct {
	Track      spotify.Track
	State      State
	Candidates []Candidate // the best first
	Chosen     int         // the candidate queued, once Matched or Accepted
}

// Job is one playlist's import, its songs in the playlist's order.
type Job struct {
	URI, Name string
	Items     []Item
}

// Count is how many of the job's songs are in state s.
func (j Job) Count(s State) int {
	n := 0
	for _, it := range j.Items {
		if it.State == s {
			n++
		}
	}
	return n
}

// Options configure the importer.
type Options struct {
	DB *sql.DB
	// Search asks YouTube Music; (*ytmusic.Client).Search is it.
	Search func(ctx context.Context, query string, f ytmusic.Filter) ([]ytmusic.Item, error)
	// Get queues downloads, State says how far one has got, and Remove takes
	// one out of the queue: the download queue's.
	Get    func(songs []ytmusic.Song)
	State  func(videoID string) (media.DownloadState, float32)
	Remove func(videoID string)
	Online func() bool
	Log    *slog.Logger
	// Pause is the wait between searches, a second unless set: YouTube
	// Music is asked at the pace a person would.
	Pause time.Duration
}

// The waits while matching can't go on: offline, and after a search failed.
const (
	offlineWait = 5 * time.Second
	failedWait  = 30 * time.Second
)

// Importer keeps the imports and matches their songs, one at a time, on a
// goroutine of its own, which also writes them to the store. Its other
// methods only touch what it holds in memory, so the screens may call them.
type Importer struct {
	o     Options
	ctx   context.Context
	stop  context.CancelFunc
	wake  chan struct{}
	ready chan struct{}
	done  chan struct{}
	now   func() time.Time
	waits [2]time.Duration // offline, and after a failed search; tests shorten them

	mu    sync.Mutex
	jobs  []*job
	err   error           // why matching stopped, until the next Import
	dirty map[string]bool // the jobs to write again
	// version changes whenever an import does, so a screen asks for one
	// again only when it has changed.
	version uint64
}

type job struct {
	uri, name string
	created   time.Time
	items     []*Item
	recheck   bool // imported again: its matched songs' files are looked for again
}

// New loads the imports from the store and starts matching.
func New(o Options) (*Importer, error) {
	if o.Pause == 0 {
		o.Pause = time.Second
	}
	ctx, stop := context.WithCancel(context.Background())
	im := &Importer{o: o, ctx: ctx, stop: stop, wake: make(chan struct{}, 1), ready: make(chan struct{}, 1),
		done: make(chan struct{}), now: time.Now, waits: [2]time.Duration{offlineWait, failedWait},
		dirty: map[string]bool{}}
	if err := im.load(); err != nil {
		stop()
		return nil, err
	}
	go im.run()
	return im, nil
}

// Ready receives a value when an import has moved on.
func (im *Importer) Ready() <-chan struct{} { return im.ready }

// Close stops matching and writes what is left to write.
func (im *Importer) Close() {
	im.stop()
	<-im.done
	im.flush()
}

func signal(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}

// Import imports playlist uri, called name, with tracks: songs new to it are
// matched, and songs no longer in it are dropped. Imported again, its
// matched songs whose files have gone are matched again.
func (im *Importer) Import(uri, name string, tracks []spotify.Track) {
	im.mu.Lock()
	defer im.mu.Unlock()
	j := im.find(uri)
	if j == nil {
		j = &job{uri: uri, created: im.now()}
		im.jobs = append(im.jobs, j)
	}
	old := map[string]*Item{}
	for _, it := range j.items {
		old[it.Track.ID] = it
	}
	listed := map[string]bool{}
	j.name, j.items, j.recheck = name, nil, true
	for _, t := range tracks {
		if t.ID == "" || listed[t.ID] {
			continue // a song twice in a playlist is imported once
		}
		listed[t.ID] = true
		it := old[t.ID]
		if it == nil {
			it = &Item{}
		}
		it.Track = t
		j.items = append(j.items, it)
	}
	im.err = nil
	im.dirty[uri] = true
	im.version++
	signal(im.wake)
}

// Job is import uri as it stands; ok is false for a playlist never imported.
func (im *Importer) Job(uri string) (Job, bool) {
	im.mu.Lock()
	defer im.mu.Unlock()
	j := im.find(uri)
	if j == nil {
		return Job{}, false
	}
	out := Job{URI: j.uri, Name: j.name, Items: make([]Item, len(j.items))}
	for i, it := range j.items {
		out.Items[i] = *it
		out.Items[i].Candidates = slices.Clone(it.Candidates)
	}
	return out, true
}

// Version changes whenever an import does.
func (im *Importer) Version() uint64 {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.version
}

// Err is why matching stopped, as when YouTube Music answers in a shape it
// can't read; nil while it goes on. Importing again tries again.
func (im *Importer) Err() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.err
}

// Use accepts candidate c of song id in import uri, and queues it.
func (im *Importer) Use(uri, id string, c int) {
	im.mu.Lock()
	it := im.item(uri, id)
	if it == nil || c < 0 || c >= len(it.Candidates) {
		im.mu.Unlock()
		return
	}
	old := ""
	if it.State == Matched || it.State == Accepted {
		old = it.Candidates[it.Chosen].Song.VideoID
	}
	it.State, it.Chosen = Accepted, c
	song := songOf(it.Track, it.Candidates[c])
	im.dirty[uri] = true
	im.version++
	im.mu.Unlock()
	if old != "" && old != song.VideoID {
		im.remove(old) // another candidate was queued for it
	}
	im.o.Get([]ytmusic.Song{song})
	signal(im.wake)
}

// Skip leaves song id of import uri out, taking it out of the download queue
// if it was queued and hasn't been saved.
func (im *Importer) Skip(uri, id string) {
	im.mu.Lock()
	it := im.item(uri, id)
	if it == nil {
		im.mu.Unlock()
		return
	}
	queued := ""
	if it.State == Matched || it.State == Accepted {
		queued = it.Candidates[it.Chosen].Song.VideoID
	}
	it.State = Skipped
	im.dirty[uri] = true
	im.version++
	im.mu.Unlock()
	if queued != "" {
		im.remove(queued)
	}
	signal(im.wake)
}

// Cancel stops import uri: songs not matched yet are skipped, and those queued
// and not saved leave the download queue.
func (im *Importer) Cancel(uri string) {
	im.mu.Lock()
	j := im.find(uri)
	if j == nil {
		im.mu.Unlock()
		return
	}
	var queued []string
	for _, it := range j.items {
		switch it.State {
		case Waiting:
			it.State = Skipped
		case Matched, Accepted:
			v := it.Candidates[it.Chosen].Song.VideoID
			if st, _ := im.o.State(v); st != media.Saved {
				queued = append(queued, v)
				it.State = Skipped
			}
		}
	}
	im.dirty[uri] = true
	im.version++
	im.mu.Unlock()
	for _, v := range queued {
		im.remove(v)
	}
	signal(im.wake)
}

// remove takes a song out of the download queue unless it has been saved.
func (im *Importer) remove(videoID string) {
	if st, _ := im.o.State(videoID); st != media.Saved {
		im.o.Remove(videoID)
	}
}

func (im *Importer) find(uri string) *job {
	for _, j := range im.jobs {
		if j.uri == uri {
			return j
		}
	}
	return nil
}

func (im *Importer) item(uri, id string) *Item {
	if j := im.find(uri); j != nil {
		for _, it := range j.items {
			if it.Track.ID == id {
				return it
			}
		}
	}
	return nil
}

// songOf is candidate c as the download of Spotify song t.
func songOf(t spotify.Track, c Candidate) ytmusic.Song {
	s := c.Song
	s.SpotifyID, s.ISRC = t.ID, t.ISRC
	return s
}

// run matches waiting songs one at a time until Close, writing the imports
// as they change.
func (im *Importer) run() {
	defer close(im.done)
	for {
		im.flush()
		im.rechecks()
		j, it := im.next()
		if it == nil {
			if !im.sleep(-1) {
				return
			}
			continue
		}
		if im.o.Online != nil && !im.o.Online() {
			if !im.sleep(im.waits[0]) {
				return
			}
			continue
		}
		wait, ok := im.match(j, it)
		if !ok || !im.sleep(wait) {
			return
		}
	}
}

// sleep waits d, or until woken when d is negative; false once closed.
func (im *Importer) sleep(d time.Duration) bool {
	var after <-chan time.Time
	if d >= 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		after = t.C
	}
	select {
	case <-after:
	case <-im.wake:
	case <-im.ctx.Done():
		return false
	}
	return true
}

// next is the first song waiting to be matched, oldest import first, unless
// matching has stopped.
func (im *Importer) next() (*job, *Item) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.err != nil {
		return nil, nil
	}
	for _, j := range im.jobs {
		for _, it := range j.items {
			if it.State == Waiting {
				return j, it
			}
		}
	}
	return nil, nil
}

// rechecks matches again the songs of imports made again whose files have
// gone since.
func (im *Importer) rechecks() {
	im.mu.Lock()
	var check []*Item
	for _, j := range im.jobs {
		if !j.recheck {
			continue
		}
		j.recheck = false
		for _, it := range j.items {
			if it.State == Matched || it.State == Accepted || it.State == Have {
				check = append(check, it)
			}
		}
		im.dirty[j.uri] = true
		im.version++
	}
	im.mu.Unlock()
	for _, it := range check {
		if !im.have(it.Track.ID) {
			im.mu.Lock()
			it.State = Waiting
			im.version++
			im.mu.Unlock()
		}
	}
}

// have reports whether the library or the download queue has Spotify song
// id already.
func (im *Importer) have(id string) bool {
	var n int
	err := im.o.DB.QueryRow(`SELECT (SELECT COUNT(*) FROM tracks WHERE spotify_id = ?) +
		(SELECT COUNT(*) FROM downloads WHERE spotify_id = ?)`, id, id).Scan(&n)
	if err != nil {
		im.o.Log.Warn("look for an imported song", "err", err)
	}
	return n > 0
}

// match matches one song: by its ISRC, then by its artist and title when that
// found nothing sure. A sure match is queued. It returns how long to wait
// before the next, and false once closed.
func (im *Importer) match(j *job, it *Item) (time.Duration, bool) {
	im.mu.Lock()
	t := it.Track
	im.mu.Unlock()
	if im.have(t.ID) {
		im.settle(j, it, Have, nil)
		return 0, true
	}
	var cands []Candidate
	if t.ISRC != "" {
		items, err := im.o.Search(im.ctx, t.ISRC, ytmusic.Songs)
		if wait, ok := im.failed(err); err != nil {
			return wait, ok
		}
		cands = rank(t, nil, items)
		if len(cands) == 0 || cands[0].Score < sureScore {
			if !im.sleep(im.o.Pause) {
				return 0, false
			}
		}
	}
	if len(cands) == 0 || cands[0].Score < sureScore {
		q := t.Title
		if len(t.Artists) > 0 {
			q = t.Artists[0] + " " + t.Title
		}
		items, err := im.o.Search(im.ctx, q, ytmusic.Songs)
		if wait, ok := im.failed(err); err != nil {
			return wait, ok
		}
		cands = rank(t, cands, items)
	}
	st := NotFound
	if len(cands) > 0 {
		st = classify(cands[0].Score)
	}
	if im.settle(j, it, st, cands) && st == Matched {
		im.o.Get([]ytmusic.Song{songOf(t, cands[0])})
	}
	return im.o.Pause, true
}

// failed deals with a search that failed: YouTube Music in a shape it can't
// read stops matching; anything else is tried again in a while. It returns
// how long to wait, and false once closed.
func (im *Importer) failed(err error) (time.Duration, bool) {
	switch {
	case err == nil:
		return 0, true
	case im.ctx.Err() != nil:
		return 0, false
	case errors.Is(err, ytmusic.ErrChanged):
		im.o.Log.Warn("import stopped", "err", err)
		im.mu.Lock()
		im.err = err
		im.version++
		im.mu.Unlock()
		signal(im.ready)
		return 0, true
	}
	im.o.Log.Warn("import search", "err", err)
	return im.waits[1], true
}

// settle gives a song its state and candidates, and reports whether it did:
// not if it was skipped meanwhile.
func (im *Importer) settle(j *job, it *Item, st State, cands []Candidate) bool {
	im.mu.Lock()
	ok := it.State == Waiting
	if ok {
		it.State, it.Candidates, it.Chosen = st, cands, 0
		im.dirty[j.uri] = true
		im.version++
	}
	im.mu.Unlock()
	signal(im.ready)
	return ok
}

// load reads the imports from the store.
func (im *Importer) load() error {
	rows, err := im.o.DB.Query(`SELECT uri, name, created FROM import_jobs ORDER BY created`)
	if err != nil {
		return err
	}
	for rows.Next() {
		j := &job{}
		var ms int64
		if err := rows.Scan(&j.uri, &j.name, &ms); err != nil {
			rows.Close()
			return err
		}
		j.created = time.UnixMilli(ms)
		im.jobs = append(im.jobs, j)
	}
	rows.Close()
	for _, j := range im.jobs {
		if err := im.loadItems(j); err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) loadItems(j *job) error {
	rows, err := im.o.DB.Query(`SELECT track, state, chosen, candidates FROM import_items WHERE uri = ? ORDER BY pos`, j.uri)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var track, cands string
		it := &Item{}
		if err := rows.Scan(&track, &it.State, &it.Chosen, &cands); err != nil {
			return err
		}
		if json.Unmarshal([]byte(track), &it.Track) != nil || json.Unmarshal([]byte(cands), &it.Candidates) != nil {
			continue
		}
		j.items = append(j.items, it)
	}
	return rows.Err()
}

// flush writes the imports that changed: each one's songs afresh.
func (im *Importer) flush() {
	type row struct {
		track, cands  []byte
		state, chosen int
		id            string
	}
	type write struct {
		uri, name string
		created   int64
		rows      []row
	}
	im.mu.Lock()
	var ws []write
	for _, j := range im.jobs {
		if !im.dirty[j.uri] {
			continue
		}
		w := write{uri: j.uri, name: j.name, created: j.created.UnixMilli()}
		for _, it := range j.items {
			track, _ := json.Marshal(it.Track)
			cands, _ := json.Marshal(it.Candidates)
			w.rows = append(w.rows, row{track: track, cands: cands, state: int(it.State), chosen: it.Chosen, id: it.Track.ID})
		}
		ws = append(ws, w)
	}
	clear(im.dirty)
	im.mu.Unlock()
	if len(ws) == 0 {
		return
	}
	err := func() error {
		tx, err := im.o.DB.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, w := range ws {
			if _, err := tx.Exec(`INSERT INTO import_jobs (uri, name, created) VALUES (?, ?, ?)
				ON CONFLICT (uri) DO UPDATE SET name = excluded.name`, w.uri, w.name, w.created); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM import_items WHERE uri = ?`, w.uri); err != nil {
				return err
			}
			for i, r := range w.rows {
				if _, err := tx.Exec(`INSERT INTO import_items (uri, spotify_id, pos, track, state, chosen, candidates)
					VALUES (?, ?, ?, ?, ?, ?, ?)`, w.uri, r.id, i, string(r.track), r.state, r.chosen, string(r.cands)); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	}()
	if err != nil {
		im.o.Log.Warn("save imports", "err", err)
	}
}
