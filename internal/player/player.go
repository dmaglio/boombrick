// Package player owns the play queue and drives the audio engine: play, pause,
// next, previous, shuffle and repeat.
package player

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ujjwal313/boombrick/internal/audio"
)

// Track is one file in the queue.
type Track struct {
	Path                              string
	Title, Artist, AlbumArtist, Album string
	Gain, Peak                        float64 // ReplayGain track gain (dB) and peak
	HasGain                           bool
	AlbumGain, AlbumPeak              float64 // ReplayGain album gain (dB) and peak
	HasAlbumGain                      bool
	Duration                          time.Duration // 0 when unknown
}

// Repeat is the repeat mode.
type Repeat uint8

const (
	RepeatOff Repeat = iota // play the queue once
	RepeatAll               // wrap to the first track after the last
	RepeatOne               // play the current track again and again
)

// State is a snapshot for the UI.
type State struct {
	Queued            bool  // the queue isn't empty
	Track             Track // the current track; zero when nothing is queued
	Index, Count      int   // position in play order, from 0, and queue length
	Playing           bool
	Elapsed, Duration time.Duration
	Rate              int    // the current file's sample rate, 0 if unknown
	Format            string // from the file extension: "MP3", "FLAC"
	Shuffle           bool
	Repeat            Repeat
	Err               error  // the last output error, which paused playback
	Version           uint64 // changes whenever the queue or its order does
}

// Opener opens a track's file for decoding.
type Opener func(path string) (audio.Source, error)

// restartAfter is how far into a track Prev restarts it instead of going back.
const restartAfter = 3 * time.Second

// levellingOffset lifts ReplayGain's -18 LUFS reference to the -14 LUFS the
// player aims for; peak protection keeps loud tracks from clipping.
const levellingOffset = 4

// Player owns the queue and drives an audio engine. It is safe for concurrent
// use: the UI calls it, and the engine reports track ends from its goroutine.
type Player struct {
	engine *audio.Engine
	open   Opener
	log    *slog.Logger

	mu      sync.Mutex
	queue   []Track
	order   []int  // play order: indexes into queue
	pos     int    // index into order
	version uint64 // bumped whenever the queue or its order changes
	cur     audio.Source
	// curRate and curFrames describe cur until the engine reports it loaded.
	curRate   int
	curFrames int64
	// next is the track primed to follow cur without a gap, at nextPos.
	next              audio.Source
	nextPos, nextRate int
	nextIdx           int  // next's entry in queue, kept right across edits
	nextAlbum         bool // whether next was levelled by album
	nextFrames        int64
	gen               uint64 // bumped whenever what should follow cur changes
	empties           int    // tracks in a row that produced no audio
	cued              bool   // cur was restored paused and hasn't been resumed
	startFrame        int64  // where cur was started, cued or last sought to
	playedOut         bool   // the queue ended and cur was dropped
	levelling, closed bool
	wg                sync.WaitGroup // priming goroutines
	shuffle           bool
	repeat            Repeat
	rng               *rand.Rand
}

// New creates a player that plays through out and opens files with open.
func New(out audio.Output, open Opener, log *slog.Logger) *Player {
	p := &Player{open: open, log: log, rng: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))}
	p.engine = audio.NewEngine(out, p.ended)
	return p
}

// Close stops playback, waits for priming to finish and releases the output.
func (p *Player) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.wg.Wait()
	return p.engine.Close()
}

// SetLevelling turns ReplayGain levelling on or off for tracks opened from now.
func (p *Player) SetLevelling(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if on == p.levelling {
		return
	}
	p.levelling = on
	if p.cur != nil {
		p.primeLocked() // the primed track was opened with the old setting
	}
}

// levelGain is the gain and peak to level t with: its album's when album is
// set and the track has one, its own otherwise.
func levelGain(t Track, album bool) (db, peak float64, ok bool) {
	switch {
	case album && t.HasAlbumGain:
		return t.AlbumGain + levellingOffset, t.AlbumPeak, true
	case t.HasGain:
		return t.Gain + levellingOffset, t.Peak, true
	}
	return 0, 0, false
}

// albumModeLocked reports whether the queue is one album played in order, so
// tracks keep their album's levels and gapless joins don't jump.
func (p *Player) albumModeLocked() bool {
	if p.shuffle || len(p.queue) == 0 {
		return false
	}
	first := p.queue[0]
	for _, t := range p.queue[1:] {
		if t.AlbumArtist != first.AlbumArtist || t.Album != first.Album {
			return false
		}
	}
	return first.Album != ""
}

func (p *Player) albumMode() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.albumModeLocked()
}

// openTrack opens t, wrapped in its ReplayGain when levelling is on, the
// album's if album is set.
func (p *Player) openTrack(t Track, levelling, album bool) (audio.Source, error) {
	src, err := p.open(t.Path)
	if err != nil {
		return nil, err
	}
	if rate := src.SampleRate(); !audio.SupportedRate(rate) {
		src.Close()
		return nil, fmt.Errorf("%w: %d Hz", audio.ErrRate, rate)
	}
	if db, peak, ok := levelGain(t, album); levelling && ok {
		src = audio.WithGain(src, db, peak)
	}
	return src, nil
}

// Play replaces the queue with tracks and starts at tracks[start].
func (p *Player) Play(tracks []Track, start int) { p.PlayFrom(tracks, start, 0) }

// PlayFrom replaces the queue with tracks and starts tracks[start] at frame,
// to carry on with an album where it was left.
func (p *Player) PlayFrom(tracks []Track, start int, frame int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.version++
	p.empties = 0 // the listener chose this; earlier silent tracks don't count
	if len(tracks) == 0 {
		p.queue, p.order, p.pos = nil, nil, 0
		p.stopLocked()
		return
	}
	p.queue = slices.Clone(tracks)
	start = min(max(start, 0), len(tracks)-1)
	p.order = p.orderLocked(start)
	p.pos = start
	if p.shuffle {
		p.pos = 0
	}
	p.startAtLocked(frame)
}

// orderLocked is the play order: the queue in order, or shuffled with first
// at the front.
func (p *Player) orderLocked(first int) []int {
	order := make([]int, len(p.queue))
	for i := range order {
		order[i] = i
	}
	if p.shuffle {
		p.rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		i := 0
		for order[i] != first {
			i++
		}
		order[0], order[i] = order[i], order[0]
	}
	return order
}

// startLocked plays the track at pos from its start.
func (p *Player) startLocked() { p.startAtLocked(0) }

// startAtLocked plays the track at pos from frame, moving on past files that
// won't open (those start from the top). It wraps at the end of the order only
// with repeat all, and stops if nothing opens.
func (p *Player) startAtLocked(frame int64) {
	p.cued, p.playedOut = false, false
	for range p.order {
		t := p.queue[p.order[p.pos]]
		src, err := p.openTrack(t, p.levelling, p.albumModeLocked())
		if err == nil {
			p.cur, p.curRate, p.curFrames, p.startFrame = src, src.SampleRate(), src.Frames(), max(frame, 0)
			p.engine.PlayFrom(src, frame)
			p.primeLocked()
			return
		}
		frame = 0
		p.log.Warn("skipping unplayable track", "path", t.Path, "err", err)
		if p.pos+1 < len(p.order) {
			p.pos++
		} else if p.repeat == RepeatAll {
			p.pos = 0
		} else {
			break
		}
	}
	p.stopLocked()
}

func (p *Player) stopLocked() {
	p.cur, p.next, p.cued = nil, nil, false
	p.startFrame = 0
	p.gen++
	p.engine.Stop()
}

// followingLocked is the position after pos in play order, if any.
func (p *Player) followingLocked(pos int) (int, bool) {
	switch {
	case p.repeat == RepeatOne:
		return pos, true
	case pos+1 < len(p.order):
		return pos + 1, true
	case p.repeat == RepeatAll:
		return 0, true
	}
	return 0, false
}

// primeLocked opens the track after cur on a goroutine of its own and queues it
// in the engine, so it follows without a gap. Anything primed before is
// dropped.
func (p *Player) primeLocked() {
	p.gen++
	p.next = nil
	p.engine.Queue(nil)
	pos, ok := p.followingLocked(p.pos)
	if !ok || p.cur == nil || p.closed {
		return
	}
	idx, gen, levelling, album := p.order[pos], p.gen, p.levelling, p.albumModeLocked()
	t := p.queue[idx]
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		src, err := p.openTrack(t, levelling, album)
		if err != nil {
			return // tried again, and skipped, when cur ends
		}
		rate, frames := src.SampleRate(), src.Frames()
		p.mu.Lock()
		defer p.mu.Unlock()
		if gen != p.gen || p.closed {
			src.Close()
			return
		}
		p.next, p.nextPos, p.nextIdx, p.nextAlbum, p.nextRate, p.nextFrames = src, pos, idx, album, rate, frames
		p.engine.Queue(src)
	}()
}

// ended moves on after a source played to its end.
func (p *Player) ended(end audio.End) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || end.Source != p.cur {
		return // replaced while it was finishing, or the player is shutting down
	}
	t := p.queue[p.order[p.pos]]
	if end.Err != nil {
		p.log.Warn("track ended early", "path", t.Path, "err", end.Err)
	}
	if end.Frames == 0 {
		p.empties++
	} else {
		p.empties = 0
	}
	if p.empties > 0 && (p.repeat == RepeatOne || p.empties >= len(p.queue)) {
		p.log.Warn("stopping: tracks play no audio", "path", t.Path)
		p.empties = 0
		p.stopLocked()
		return
	}
	if end.Next != nil && end.Next == p.next {
		// The primed track took over without a gap.
		p.cur, p.pos, p.curRate, p.curFrames, p.startFrame = p.next, p.nextPos, p.nextRate, p.nextFrames, 0
		p.primeLocked()
		return
	}
	pos, ok := p.followingLocked(p.pos)
	if !ok {
		p.cur, p.next, p.playedOut = nil, nil, true // the queue stays, stopped on its last track
		return
	}
	p.pos = pos
	p.startLocked()
}

// Toggle pauses or resumes, or plays the current track again once the queue
// has played out.
func (p *Player) Toggle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case len(p.queue) == 0:
	case p.cur == nil:
		p.startLocked()
	case p.playingLocked():
		p.engine.Pause()
	default:
		p.cued = false
		p.engine.Resume()
	}
}

// Next skips to the next track, wrapping at the end only with repeat all.
func (p *Player) Next() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.empties = 0
	switch {
	case len(p.queue) == 0:
		return
	case p.pos+1 < len(p.order):
		p.pos++
	case p.repeat == RepeatAll:
		p.pos = 0
	default:
		return
	}
	p.startLocked()
}

// Prev goes back a track, or restarts the current one once it has played for
// a few seconds or when it is the first.
func (p *Player) Prev() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.empties = 0
	if len(p.queue) == 0 {
		return
	}
	if p.pos > 0 && (p.cur == nil || p.elapsedLocked() <= restartAfter) {
		p.pos--
		p.startLocked()
		return
	}
	p.startLocked()
}

// Seek moves the current track to d from its start, clamped to its length.
// It does nothing when no track is loaded.
func (p *Player) Seek(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cur == nil || p.curRate <= 0 {
		return
	}
	f := max(int64(d)*int64(p.curRate)/int64(time.Second), 0)
	if p.curFrames > 0 {
		f = min(f, p.curFrames)
	}
	p.startFrame = f
	p.engine.SeekFrame(f)
}

// SetShuffle turns shuffle on or off without interrupting the current track.
func (p *Player) SetShuffle(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if on == p.shuffle {
		return
	}
	p.shuffle = on
	if len(p.queue) == 0 {
		return
	}
	cur := p.order[p.pos]
	p.order = p.orderLocked(cur)
	p.pos = cur
	if on {
		p.pos = 0
	}
	p.editedLocked()
}

// CycleRepeat steps through off, all and one.
func (p *Player) CycleRepeat() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.repeat = (p.repeat + 1) % 3
	if p.cur != nil {
		p.primeLocked()
	}
}

// Enqueue adds tracks to the end of the play order. Nothing starts playing if
// nothing was queued; Toggle starts the first. After the queue has played out,
// Toggle starts the first track added.
func (p *Player) Enqueue(tracks []Track) {
	if len(tracks) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.queue)
	if p.playedOut {
		// Toggle starts the new music rather than the old last track.
		p.pos, p.playedOut = len(p.order), false
	}
	p.queue = append(p.queue, tracks...)
	for i := range tracks {
		p.order = append(p.order, n+i)
	}
	p.editedLocked()
}

// Queue appends the queue in play order to dst and returns it, with the
// current track's position in it.
func (p *Player) Queue(dst []Track) ([]Track, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, i := range p.order {
		dst = append(dst, p.queue[i])
	}
	return dst, p.pos
}

// PlayIndex plays the track at position i of the play order.
func (p *Player) PlayIndex(i int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i < 0 || i >= len(p.order) {
		return
	}
	p.pos, p.empties = i, 0
	p.startLocked()
}

// Remove takes the track at position i of the play order out of the queue.
// The current track can't be removed.
func (p *Player) Remove(i int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i < 0 || i >= len(p.order) || i == p.pos {
		return
	}
	q := p.order[i]
	switch {
	case q == p.nextIdx:
		p.nextIdx = -1 // the primed entry is gone
	case q < p.nextIdx:
		p.nextIdx--
	}
	p.queue = slices.Delete(p.queue, q, q+1)
	p.order = slices.Delete(p.order, i, i+1)
	for j, v := range p.order {
		if v > q {
			p.order[j] = v - 1
		}
	}
	if i < p.pos {
		p.pos--
	}
	p.editedLocked()
}

// Move moves the track at position from of the play order to position to.
// Only tracks after the current one move, and only among themselves.
func (p *Player) Move(from, to int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.order)
	if from == to || from <= p.pos || to <= p.pos || from >= n || to >= n {
		return
	}
	v := p.order[from]
	p.order = slices.Insert(slices.Delete(p.order, from, from+1), to, v)
	p.editedLocked()
}

// editedLocked records a change to the queue. The track to follow the current
// one is primed again unless it is the entry already primed, at the same
// levels, in which case only its position is updated.
func (p *Player) editedLocked() {
	p.version++
	if p.cur == nil {
		return
	}
	if p.next != nil {
		if pos, ok := p.followingLocked(p.pos); ok && p.order[pos] == p.nextIdx && p.albumModeLocked() == p.nextAlbum {
			p.nextPos = pos
			return
		}
	}
	p.primeLocked()
}

// State reports what is playing.
func (p *Player) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := State{Shuffle: p.shuffle, Repeat: p.repeat, Count: len(p.queue), Version: p.version}
	if len(p.queue) == 0 {
		return s
	}
	t := p.queue[p.order[p.pos]]
	s.Queued, s.Track, s.Index = true, t, p.pos
	s.Format = strings.ToUpper(strings.TrimPrefix(filepath.Ext(t.Path), "."))
	if p.cur == nil {
		return s
	}
	st := p.engine.Status()
	if st.Source != p.cur {
		// The engine hasn't picked the new source up yet.
		s.Playing, s.Rate = !p.cued, p.curRate
		if p.curRate > 0 {
			s.Elapsed = duration(p.startFrame, p.curRate)
			if p.curFrames > 0 {
				s.Duration = duration(p.curFrames, p.curRate)
			}
		}
		return s
	}
	s.Playing, s.Rate, s.Err = st.Playing, st.Rate, st.Err
	if st.Rate > 0 {
		frame := max(st.Frame, p.startFrame) // the engine hasn't reached the start yet
		s.Elapsed = duration(frame, st.Rate)
		if st.Frames > 0 {
			s.Duration = duration(st.Frames, st.Rate)
		}
	}
	return s
}

// playingLocked reports whether cur is playing. A source the engine hasn't
// picked up yet counts as playing, as Play starts it unpaused, unless it was
// restored paused.
func (p *Player) playingLocked() bool {
	st := p.engine.Status()
	return st.Source != p.cur && !p.cued || st.Source == p.cur && st.Playing
}

func (p *Player) elapsedLocked() time.Duration {
	st := p.engine.Status()
	if st.Source != p.cur || st.Rate == 0 {
		return 0
	}
	return duration(st.Frame, st.Rate)
}

func duration(frames int64, rate int) time.Duration {
	return time.Duration(frames) * time.Second / time.Duration(rate)
}

// Snapshot is what is saved to carry on after a restart.
type Snapshot struct {
	Queue    []Track
	Order    []int
	Pos      int
	Frame    int64   // position in the current track, in its frames
	Fraction float64 // how far through the current track's album, 0 to 1
	// AlbumRun is set when the current track's album was queued as an album:
	// its tracks sit together in the queue, and there is more than one.
	// Fraction says nothing about a song queued from a list of songs.
	AlbumRun bool
	Shuffle  bool
	Repeat   Repeat
	Version  uint64 // changes whenever the queue or its order does
}

// Snapshot reports the queue and position; ok is false when nothing is queued.
func (p *Player) Snapshot() (Snapshot, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) == 0 {
		return Snapshot{}, false
	}
	s := Snapshot{Queue: slices.Clone(p.queue), Order: slices.Clone(p.order), Pos: p.pos, Shuffle: p.shuffle, Repeat: p.repeat, Version: p.version}
	var elapsed, total float64
	if st := p.engine.Status(); p.cur != nil && st.Source == p.cur {
		s.Frame = st.Frame
		if st.Frames > 0 {
			elapsed, total = float64(st.Frame), float64(st.Frames)
		}
	}
	if (p.cur != nil || p.cued) && s.Frame < p.startFrame {
		// Started, cued or sought, and the engine hasn't got there yet.
		s.Frame = p.startFrame
		if p.curFrames > 0 {
			elapsed, total = float64(s.Frame), float64(p.curFrames)
		}
	}
	s.AlbumRun = p.albumRunLocked()
	if p.playedOut {
		s.Frame, s.Fraction = max(p.curFrames, 0), 1
		return s, true
	}
	s.Fraction = p.albumFractionLocked(elapsed, total)
	return s, true
}

// sameAlbum reports whether t and u are tracks of one album.
func sameAlbum(t, u Track) bool {
	return t.Album != "" && t.AlbumArtist == u.AlbumArtist && t.Album == u.Album
}

// albumFractionLocked is how far through the current track's album the
// listener is, counting that album's tracks played before it, in play order,
// so a shuffled album moves on as it plays. A track without an album is one
// of its own.
func (p *Player) albumFractionLocked(elapsed, total float64) float64 {
	t := p.queue[p.order[p.pos]]
	before, n := 0, 1
	if t.Album != "" {
		n = 0
		for _, q := range p.queue {
			if sameAlbum(t, q) {
				n++
			}
		}
		for _, i := range p.order[:p.pos] {
			if sameAlbum(t, p.queue[i]) {
				before++
			}
		}
	}
	f := float64(before)
	if total > 0 {
		f += elapsed / total
	}
	return f / float64(n)
}

// albumRunLocked reports whether the current track's album is one unbroken
// run of more than one entry in the queue, as queueing an album leaves it.
func (p *Player) albumRunLocked() bool {
	t := p.queue[p.order[p.pos]]
	first, last, n := -1, -1, 0
	for i, q := range p.queue {
		if sameAlbum(t, q) {
			if first < 0 {
				first = i
			}
			last = i
			n++
		}
	}
	return n > 1 && last-first+1 == n
}

// Restore loads a saved snapshot paused at its position, unless something has
// been queued since launch.
func (p *Player) Restore(s Snapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) > 0 || len(s.Queue) == 0 || len(s.Order) != len(s.Queue) || s.Pos < 0 || s.Pos >= len(s.Order) {
		return
	}
	for _, i := range s.Order {
		if i < 0 || i >= len(s.Queue) {
			return
		}
	}
	p.queue, p.order, p.pos = slices.Clone(s.Queue), slices.Clone(s.Order), s.Pos
	p.shuffle, p.repeat = s.Shuffle, s.Repeat
	p.version++
	t := p.queue[p.order[p.pos]]
	// Kept even if the track won't open, so the next save doesn't lose the position.
	p.cued, p.startFrame, p.playedOut = true, max(s.Frame, 0), false
	src, err := p.openTrack(t, p.levelling, p.albumModeLocked())
	if err != nil {
		p.log.Warn("can't resume track", "path", t.Path, "err", err)
		return // the queue is back, stopped
	}
	p.cur, p.curRate, p.curFrames = src, src.SampleRate(), src.Frames()
	p.engine.Cue(src, s.Frame)
	p.primeLocked()
}
