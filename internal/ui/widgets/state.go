package widgets

import (
	"strings"

	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/text"
)

var (
	stateTitle  = text.Style{Face: text.CormorantSemiBold, Size: 48}
	stateBody   = text.Body
	stateDetail = text.Meta
)

// stateWidth is how wide a state's description runs before wrapping.
const stateWidth = 600

// Wrap breaks s at spaces into lines no wider than w; a single word wider
// than w gets a line of its own.
func Wrap(u *ui.Context, s string, st text.Style, w float32) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if line != "" && u.TextWidth(next, st) > w {
			lines = append(lines, line)
			line = word
			continue
		}
		line = next
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// Wrapped keeps the lines of the last text wrapped, so a screen that shows
// the same text frame after frame wraps it once.
type Wrapped struct {
	text  string
	style text.Style
	w     float32
	scale float32
	lines []string
}

// Lines returns s wrapped to w, wrapping again only when s, st, w or the
// context's scale changes.
func (c *Wrapped) Lines(u *ui.Context, s string, st text.Style, w float32) []string {
	// The zero Wrapped has no scale, so the first call always wraps.
	if s != c.text || st != c.style || w != c.w || u.Scale != c.scale {
		c.text, c.style, c.w, c.scale = s, st, w, u.Scale
		c.lines = Wrap(u, s, st, w)
	}
	return c.lines
}

// EmptyState draws a state screen's message centred between the tabs and the
// hint bar: the logo mark in ink4, a title, a wrapped
// description in ink2 and a detail line in ink3. The screen keeps the
// description's lines in c, so that it isn't wrapped every frame; with c nil
// it is.
func EmptyState(u *ui.Context, c *Wrapped, title, body, detail string) {
	const mark = 54 // em: the mark is 81 x 69
	var lines []string
	if c != nil {
		lines = c.Lines(u, body, stateBody, stateWidth)
	} else {
		lines = Wrap(u, body, stateBody, stateWidth)
	}
	titleH, bodyH, detailH := u.LineHeight(stateTitle), u.LineHeight(stateBody), u.LineHeight(stateDetail)
	h := 1.27*mark + 24 + titleH + 12 + float32(len(lines))*bodyH + 16 + detailH
	top := float32(StatusHeight + TabsHeight)
	y := top + (theme.ScreenH-HintsHeight-top-h)/2
	LogoMark(u, mark, (theme.ScreenW-1.5*mark)/2, y, theme.Ink4)
	y += 1.27*mark + 24
	u.Text(title, stateTitle, (theme.ScreenW-u.TextWidth(title, stateTitle))/2, y, theme.Paper)
	y += titleH + 12
	for _, l := range lines {
		u.Text(l, stateBody, (theme.ScreenW-u.TextWidth(l, stateBody))/2, y, theme.Ink2)
		y += bodyH
	}
	y += 16
	u.Text(detail, stateDetail, (theme.ScreenW-u.TextWidth(detail, stateDetail))/2, y, theme.Ink3)
}
