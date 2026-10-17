// Package browse is the Library tab and the album and artist pages it opens
// (docs/ui.md, screen 04).
package browse

import (
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/screens"
)

// Player is what browsing needs from the player.
type Player interface {
	State() player.State
	Play(tracks []player.Track, start int)
	SetShuffle(on bool)
	Enqueue(tracks []player.Track)
}

// The tabs' page layout (screens/layout.go), by the short names these
// screens use.
const (
	contentTop = screens.ContentTop
	contentBot = screens.ContentBot
	side       = screens.Side
	pad        = screens.Pad
	gridArt    = screens.GridArt
	gridX      = screens.GridX
)

// artistLine is an artist's album and song counts.
func artistLine(albums, songs int) string {
	a := screens.Count(albums) + " albums"
	if albums == 1 {
		a = "1 album"
	}
	s := screens.Count(songs) + " songs"
	if songs == 1 {
		s = "1 song"
	}
	return a + " · " + s
}
