package audio

import (
	"errors"
	"io"
	"sync"

	"github.com/ujjwal313/boombrick/internal/audio/resample"
)

// OutputRate is the rate the device runs at; every source is resampled to it.
const OutputRate = 48000

const (
	// chunk is how many source frames each read carries: about 21 ms at 48 kHz.
	chunk = 1024
	// maxRatio bounds how many output frames one source frame becomes (8 kHz
	// to 48 kHz).
	maxRatio = 6
	// history is how many written output frames the engine keeps, so what a
	// pause drops from the device can be played again on resume. It covers the
	// device's 100 ms buffer with room to spare.
	history = OutputRate / 4
	// resampleQuality trades CPU for accuracy on speexdsp's 0-10 scale.
	resampleQuality = 4
)

// Source rates the engine accepts: maxRatio sizes its buffers for 8 kHz.
const (
	MinRate = 8000
	MaxRate = 384000
)

// ErrRate is reported for sources whose sample rate the engine can't play.
var ErrRate = errors.New("audio: unsupported sample rate")

// SupportedRate reports whether the engine can play a source at rate.
func SupportedRate(rate int) bool { return rate >= MinRate && rate <= MaxRate }

// Status is a snapshot of the engine.
type Status struct {
	Loaded  bool   // a source is playing or paused
	Playing bool   // loaded and not paused
	Frame   int64  // the source's position, in its own frames
	Frames  int64  // the source's length, or -1
	Rate    int    // the source's sample rate
	Err     error  // the last output error; it pauses playback
	Source  Source // the loaded source, for identity checks only; nil when stopped
}

// End describes a source that finished.
type End struct {
	Source Source // the source that played out, now closed
	Next   Source // the queued source that took over without a gap, or nil
	Frames int64  // the position it ended at, in its own frames (a Cue offset counts)
	Err    error  // the read error that ended it early, or nil at a clean end
}

// Engine plays one Source at a time through an Output at OutputRate. A single
// goroutine reads a chunk, resamples it, converts it to 16-bit and writes it;
// the output's own buffer absorbs scheduling hiccups. Its buffers are
// allocated once.
type Engine struct {
	out   Output
	onEnd func(End)
	done  chan struct{}

	mu       sync.Mutex
	wake     *sync.Cond
	cur      Source // read by the loop; set only by the loop, under mu
	next     Source // handed over by Play or Cue, swapped in by the loop
	queued   Source // follows cur without a gap
	switched bool
	paused   bool
	seek     int64 // pending seek, or -1
	quiet    bool  // nothing has been written since the output last went idle
	replay   int   // output frames from history to write again before reading on
	back     int64 // source frames the pause took off Frame, which the replay restores
	closed   bool
	st       Status

	// Ends wait in a FIFO for a notifier goroutine, so onEnd sees them in the
	// order they happened and the loop never blocks on it.
	nmu     sync.Mutex
	nwake   *sync.Cond
	pending []End
	stopped bool // the loop has exited
}

// NewEngine starts the playback goroutine. onEnd runs each time a source plays
// to its end, on a goroutine of its own, one End at a time, in the order
// sources ended.
func NewEngine(out Output, onEnd func(End)) *Engine {
	e := &Engine{out: out, onEnd: onEnd, seek: -1, done: make(chan struct{}), st: Status{Frames: -1}}
	e.wake = sync.NewCond(&e.mu)
	e.nwake = sync.NewCond(&e.nmu)
	go e.loop()
	if onEnd != nil {
		go e.notify()
	}
	return e
}

// notify hands queued Ends to onEnd one at a time. Separate goroutines per End
// could overtake each other: a queued source that ends at once reports right
// behind the End that handed over to it.
func (e *Engine) notify() {
	e.nmu.Lock()
	for {
		for len(e.pending) == 0 {
			if e.stopped {
				e.nmu.Unlock()
				return
			}
			e.nwake.Wait()
		}
		end := e.pending[0]
		e.pending[0] = End{}
		e.pending = e.pending[1:]
		e.nmu.Unlock()
		e.onEnd(end)
		e.nmu.Lock()
	}
}

func (e *Engine) report(end End) {
	if e.onEnd == nil {
		return
	}
	e.nmu.Lock()
	e.pending = append(e.pending, end)
	e.nmu.Unlock()
	e.nwake.Signal()
}

// Play replaces whatever is playing with src, unpaused; nil stops. Anything
// queued is dropped. The engine closes sources once it is done with them.
func (e *Engine) Play(src Source) { e.load(src, false, -1) }

// PlayFrom is Play starting at frame. The seek happens before the first read,
// so nothing from the start of the source is heard. From frame 0 it is Play:
// a seek would drop what the device still holds of a track that played out.
func (e *Engine) PlayFrom(src Source, frame int64) {
	if frame <= 0 {
		frame = -1
	}
	e.load(src, false, frame)
}

// Cue loads src paused at frame, to be started with Resume.
func (e *Engine) Cue(src Source, frame int64) { e.load(src, true, max(frame, 0)) }

func (e *Engine) load(src Source, paused bool, seek int64) {
	if src != nil && !SupportedRate(src.SampleRate()) {
		closeSource(src)
		src = nil // callers check SupportedRate first; this only guards the buffers
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		closeSource(src)
		return
	}
	next, queued := e.next, e.queued // handed over but never started
	e.next, e.queued, e.switched, e.paused, e.seek = src, nil, true, paused, seek
	e.mu.Unlock()
	if next != src {
		closeSource(next)
	}
	if queued != src {
		closeSource(queued)
	}
	e.wake.Signal()
}

// Queue sets src to follow the current source without a gap, replacing and
// closing any queued before; nil clears it. The current source can't follow
// itself, so it is ignored.
func (e *Engine) Queue(src Source) {
	if src != nil && !SupportedRate(src.SampleRate()) {
		closeSource(src)
		src = nil
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		closeSource(src)
		return
	}
	if src != nil && (src == e.cur || src == e.next) {
		e.mu.Unlock()
		return
	}
	old := e.queued
	e.queued = src
	e.mu.Unlock()
	if old != src {
		closeSource(old)
	}
}

// Stop stops playback.
func (e *Engine) Stop() { e.Play(nil) }

// Pause holds playback. What the device still held is played again on Resume.
func (e *Engine) Pause() { e.set(func() { e.paused = true }) }

// Resume continues after Pause or Cue, or after an output error paused
// playback.
func (e *Engine) Resume() { e.set(func() { e.paused, e.st.Err = false, nil }) }

// SeekFrame moves the current source to frame.
func (e *Engine) SeekFrame(frame int64) { e.set(func() { e.seek = max(frame, 0) }) }

func (e *Engine) set(f func()) {
	e.mu.Lock()
	f()
	e.mu.Unlock()
	e.wake.Signal()
}

// Status reports what the engine is doing.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.st
	st.Playing = st.Loaded && !e.paused
	return st
}

// Close stops the goroutine, closes any source and closes the output.
func (e *Engine) Close() error {
	e.set(func() { e.closed = true })
	<-e.done
	return e.out.Close()
}

func (e *Engine) loop() {
	defer close(e.done)
	defer func() {
		e.nmu.Lock()
		e.stopped = true
		e.nmu.Unlock()
		e.nwake.Broadcast()
	}()
	rs := resample.New(2, resampleQuality)
	defer rs.Close()
	in := make([]float32, 2*chunk)
	out := make([]float32, 2*chunk*maxRatio+64)
	pcm := make([]int16, len(out))
	hist := newRing(2 * history)
	again := make([]int16, 2*history)
	for {
		e.mu.Lock()
		switch {
		case e.closed:
			srcs := []Source{e.cur, e.next, e.queued}
			e.cur, e.next, e.queued = nil, nil, nil
			e.mu.Unlock()
			for _, s := range srcs {
				closeSource(s)
			}
			return
		case e.switched:
			old, cur := e.cur, e.next
			e.cur, e.next, e.switched, e.replay, e.back = cur, nil, false, 0, 0
			e.st = Status{Loaded: cur != nil, Frames: -1, Source: cur}
			if cur != nil {
				e.st.Frames, e.st.Rate = cur.Frames(), cur.SampleRate()
			}
			e.mu.Unlock()
			rs.Reset()
			hist.clear()
			// A source that ended has already left the device to play out;
			// only an interrupted one is cut off. One handed over again is
			// still in use.
			if old != nil {
				if old != cur {
					closeSource(old)
				}
				e.drop()
			}
			continue
		case e.seek >= 0 && e.cur != nil:
			cur, frame := e.cur, e.seek
			e.seek, e.replay, e.back = -1, 0, 0
			e.mu.Unlock()
			err := cur.SeekFrame(frame)
			rs.Reset()
			hist.clear()
			e.drop()
			if err == nil {
				e.mu.Lock()
				if e.cur == cur {
					e.st.Frame = frame
				}
				e.mu.Unlock()
			}
			continue
		case e.cur == nil || e.paused:
			if e.paused && !e.quiet {
				e.mu.Unlock()
				dropped, _ := e.out.Drop()
				e.mu.Lock()
				// The dropped audio is written again on resume, so pausing
				// doesn't skip it.
				e.replay = min(dropped, hist.len()/2)
				// Close to the start of a track the replay can reach back
				// further than the track goes; only what was taken off is
				// put back.
				e.back = 0
				if e.st.Rate > 0 {
					e.back = min(e.st.Frame, int64(e.replay)*int64(e.st.Rate)/OutputRate)
					e.st.Frame -= e.back
				}
				e.quiet = true
				e.mu.Unlock()
				continue
			}
			e.quiet = true
			e.wake.Wait()
			e.mu.Unlock()
			continue
		}
		cur, rate, replay, back := e.cur, e.st.Rate, e.replay, e.back
		e.replay, e.back, e.quiet = 0, 0, false
		e.mu.Unlock()

		if replay > 0 {
			werr := e.write(hist.tail(again[:2*replay]))
			e.mu.Lock()
			if e.cur == cur && !e.switched {
				if werr != nil {
					// Nothing was replayed: read on from the rewound frame.
					e.paused, e.st.Err = true, werr
					if e.seek < 0 {
						e.seek = e.st.Frame
					}
				} else {
					e.st.Frame += back
				}
			}
			e.mu.Unlock()
			continue
		}

		n, rerr := cur.Read(in)
		var werr error
		if n > 0 {
			k := rs.Process(rate, OutputRate, in[:2*n], out)
			toInt16(pcm[:2*k], out[:2*k])
			if werr = e.write(pcm[:2*k]); werr == nil {
				hist.add(pcm[:2*k])
			}
		}

		e.mu.Lock()
		if e.cur != cur || e.switched {
			e.mu.Unlock()
			continue
		}
		if werr != nil {
			// The chunk didn't play: rewind to it, so Resume plays it. A seek
			// the caller asked for meanwhile wins.
			e.paused, e.st.Err = true, werr
			if e.seek < 0 {
				e.seek = e.st.Frame
			}
			e.mu.Unlock()
			continue
		}
		e.st.Frame += int64(n)
		if rerr == nil {
			e.mu.Unlock()
			continue
		}
		end := End{Source: cur, Frames: e.st.Frame}
		if !errors.Is(rerr, io.EOF) {
			end.Err = rerr
		}
		reset := false
		if q := e.queued; q != nil {
			// Gapless: the queued source carries straight on through the same
			// resampler and device buffer. A seek aimed at the old source
			// doesn't carry over.
			reset = q.SampleRate() != rate
			e.cur, e.queued, end.Next = q, nil, q
			e.seek, e.back = -1, 0
			e.st = Status{Loaded: true, Frames: q.Frames(), Rate: q.SampleRate(), Source: q}
		} else {
			e.cur = nil
			e.st.Loaded, e.st.Source = false, nil
		}
		e.mu.Unlock()
		closeSource(cur)
		if reset {
			// Switching the running filter to a new rate leaves it holding
			// extra output from the old, longer filter; after a high rate
			// down to 8 kHz that overflows out. Same-rate hand-overs keep the
			// filter, so they stay seamless.
			rs.Reset()
		}
		e.report(end)
	}
}

// write opens the device if needed and writes pcm.
func (e *Engine) write(pcm []int16) error {
	if err := e.out.Open(OutputRate); err != nil {
		return err
	}
	return e.out.Write(pcm)
}

// drop discards audio queued in the output and marks it quiet.
func (e *Engine) drop() {
	e.out.Drop()
	e.mu.Lock()
	e.quiet = true
	e.mu.Unlock()
}

func closeSource(s Source) {
	if s != nil {
		s.Close()
	}
}

func toInt16(dst []int16, src []float32) {
	for i, v := range src {
		dst[i] = int16(max(-1, min(1, v)) * 32767)
	}
}
