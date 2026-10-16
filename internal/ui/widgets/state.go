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

// EmptyState draws a state screen's message centred between the tabs and the
// hint bar (docs/ui.md, screens 12): the logo mark in ink4, a title, a wrapped
// description in ink2 and a detail line in ink3.
func EmptyState(u *ui.Context, title, body, detail string) {
	const mark = 54 // em: the mark is 81 x 69
	lines := Wrap(u, body, stateBody, stateWidth)
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
