// Package screens defines the Screen interface and screens shared by tabs.
package screens

import (
	"time"

	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/text"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// Screen is one full frame of UI.
type Screen interface {
	// Handle applies a button event and reports whether the frame changed.
	Handle(ev input.Event, now time.Time) bool
	// Draw renders the whole frame.
	Draw(u *ui.Context)
	// Animating reports whether another frame is needed at now even without input.
	Animating(now time.Time) bool
}

// Ticker is a screen that shows the playing track's time and wants a frame
// that often while music plays. Screens without it get one a second. Tick
// returns a positive duration; the app divides elapsed time by it.
type Ticker interface {
	Tick() time.Duration
}

// DrawReading draws the state Home and the Library show while the first scan
// runs and nothing is indexed yet.
func DrawReading(u *ui.Context, musicDir string) {
	widgets.DrawBackground(u, widgets.StateGround)
	widgets.EmptyState(u, "Reading your music", "This takes a little while the first time.", musicDir)
}

// Placeholder stands in for a tab that hasn't been built yet.
type Placeholder struct {
	Tab   int
	Title string
}

// Handle ignores input.
func (p *Placeholder) Handle(input.Event, time.Time) bool { return false }

// Animating is always false.
func (p *Placeholder) Animating(time.Time) bool { return false }

// Draw shows the tab chrome and the tab's name.
func (p *Placeholder) Draw(u *ui.Context) {
	widgets.DrawBackground(u, widgets.TabGround)
	widgets.StatusBar(u)
	widgets.Tabs(u, p.Tab)
	u.Text(p.Title, text.Heading, 32, widgets.StatusHeight+widgets.TabsHeight+14, theme.Paper)
	widgets.HintBar(u, []widgets.Hint{{Key: "L1/R1", Label: "Tabs"}})
}
