// Package browse is the Library tab and the album and artist pages it opens
// (docs/ui.md, screen 04).
package browse

import (
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/text"
	"github.com/ujjwal313/boombrick/internal/ui/views"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// Player is what browsing needs from the player.
type Player interface {
	State() player.State
	Play(tracks []player.Track, start int)
	SetShuffle(on bool)
	Enqueue(tracks []player.Track)
}

// Layout shared by the browse screens, in design pixels.
const (
	contentTop = widgets.StatusHeight + widgets.TabsHeight
	contentBot = theme.ScreenH - widgets.HintsHeight
	side       = 32
	// The album grid: three 212 px cards, 36 apart across and 28 down,
	// centred on the canvas.
	gridArt  = 212
	gridGapX = 36
	gridGapY = 28
	gridX    = (theme.ScreenW - 3*gridArt - 2*gridGapX) / 2
	pad      = 14 // room above a list or grid for the focus ring
)

// Lists and grids clip their items at their own tops, so scrolled ones don't
// show over what is above. Focus glows reach further, over everything between
// the status bar and the hint bar, so they fade out instead of ending at an
// edge: over a heading, the filters, or the tabs above the album page's first
// track.
var glowArea = gfx.Rect{X: 0, Y: widgets.StatusHeight, W: theme.ScreenW, H: contentBot - widgets.StatusHeight}

// below is the part of the content from y down, where a list or grid starting
// at y draws its items.
func below(y float32) gfx.Rect { return gfx.Rect{X: 0, Y: y, W: theme.ScreenW, H: contentBot - y} }

// albumGrid is the grid of album cards; layoutGrid fills in what depends on
// the fonts.
func albumGrid() views.Grid {
	return views.Grid{Cols: 3, CellW: gridArt, GapX: gridGapX, GapY: gridGapY, Pad: pad}
}

func layoutGrid(u *ui.Context, g *views.Grid, viewH float32) {
	g.CellH, g.ViewH = widgets.CardHeight(u, gridArt), viewH
}

// heading draws a Heading on the left and a Meta count on the right, as Home's
// rows do.
func heading(u *ui.Context, title, meta string, y float32) {
	u.Text(u.Fit(title, text.Heading, theme.ScreenW-2*side-u.TextWidth(meta, text.Meta)-24), text.Heading, side, y, theme.Paper)
	baseline := y + u.Ascent(text.Heading)
	u.Text(meta, text.Meta, theme.ScreenW-side-u.TextWidth(meta, text.Meta), u.Baseline(text.Meta, baseline), theme.Ink3)
}

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
