package app

import (
	"slices"
	"testing"

	"github.com/ujjwal313/boombrick/internal/library"
	"github.com/ujjwal313/boombrick/internal/player"
)

// albumCalls records what starting an album asks of the player.
type albumCalls struct {
	shuffle      []bool
	tracks       int
	start        int
	frame        int64
	shuffledPlay bool // PlayFrom came while shuffle was on
}

func (c *albumCalls) SetShuffle(on bool) { c.shuffle = append(c.shuffle, on) }
func (c *albumCalls) PlayFrom(ts []player.Track, start int, frame int64) {
	c.tracks, c.start, c.frame = len(ts), start, frame
	c.shuffledPlay = len(c.shuffle) == 0 || c.shuffle[len(c.shuffle)-1]
}

// Home's Play and Continue listening start albums in order, whatever shuffle
// an album page left on.
func TestAlbumsFromHomePlayInOrder(t *testing.T) {
	al := &library.Album{Artist: "Odalys", Title: "Copper Season", Tracks: []library.Track{{Path: "/a/1"}, {Path: "/a/2"}}}
	for _, c := range []struct {
		track int
		frame int64
	}{{0, 0}, {1, 4800}} {
		p := &albumCalls{shuffle: []bool{true}}
		playAlbum(p, al, c.track, c.frame)
		if !slices.Equal(p.shuffle, []bool{true, false}) || p.shuffledPlay {
			t.Fatalf("shuffle set %v before playing; want it off first", p.shuffle)
		}
		if p.tracks != 2 || p.start != c.track || p.frame != c.frame {
			t.Fatalf("played %d tracks from %d at %d, want 2 from %d at %d", p.tracks, p.start, p.frame, c.track, c.frame)
		}
	}
}
