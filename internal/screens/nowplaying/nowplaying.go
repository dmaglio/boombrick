// Package nowplaying is the Now Playing screen (docs/ui.md, screen 02).
package nowplaying

import (
	"fmt"
	"image/color"
	"time"

	"github.com/ujjwal313/boombrick/internal/media"
	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/player"
	"github.com/ujjwal313/boombrick/internal/theme"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/gfx"
	"github.com/ujjwal313/boombrick/internal/ui/raster"
	"github.com/ujjwal313/boombrick/internal/ui/text"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// Player is what the screen needs from the player.
type Player interface {
	State() player.State
	Toggle()
	Next()
	Prev()
	SetShuffle(on bool)
	CycleRepeat()
}

// Controls, left to right.
const (
	shuffleCtl = iota
	prevCtl
	playCtl
	nextCtl
	repeatCtl
	controlCount
)

// controlOffset is each control's centre relative to the column's centre, in
// design pixels. Fixed offsets keep prev and next clear of the 108 px
// play/pause ring and its focus ring.
var controlOffset = [controlCount]float32{-170, -100, 0, 100, 170}

var controlIcons = [controlCount]string{"shuffle", "prev", "play", "next", "repeat"}

// Layout in design pixels: docs/ui.md, screen 02, fitted to the canvas.
const (
	side     = 44
	artSize  = 300
	colGap   = 44
	groupGap = 26
	ctlRow   = 108 // the play/pause circle
	ctlFocus = 64  // focus circle around the other controls
	knob     = 18
)

var (
	titleStyle  = text.Style{Face: text.CormorantSemiBold, Size: 52}
	artistStyle = text.Style{Face: text.Lora, Size: 30}
	albumStyle  = text.Style{Face: text.LoraItalic, Size: 24}
	timeStyle   = text.Style{Face: text.Lora, Size: 22, Tabular: true}
	formatStyle = text.Style{Face: text.Lora, Size: 20}
	oneStyle    = text.Style{Face: text.LoraSemiBold, Size: 15}
)

// NowPlaying shows the current track and its transport controls.
type NowPlaying struct {
	p      Player
	focus  int
	values [controlCount]anim.Value // focus strength per control
}

// New creates the screen with focus on play/pause.
func New(p Player) *NowPlaying {
	n := &NowPlaying{p: p, focus: playCtl}
	n.values[playCtl].Set(1)
	return n
}

// Handle moves focus along the controls, activates them with A and changes
// track with L1 and R1. B is handled by the app, which closes the screen.
func (n *NowPlaying) Handle(ev input.Event, now time.Time) bool {
	if ev.Kind == input.Release {
		return false
	}
	switch ev.Button {
	case input.Left, input.Right:
		f := n.focus - 1
		if ev.Button == input.Right {
			f = n.focus + 1
		}
		f = min(max(f, 0), controlCount-1)
		if f == n.focus {
			return false
		}
		n.values[n.focus].To(0, now, anim.Focus, anim.EaseOutCubic)
		n.focus = f
		n.values[f].To(1, now, anim.Focus, anim.EaseOutCubic)
		return true
	case input.A:
		if ev.Kind != input.Press {
			return false
		}
		n.activate()
		return true
	case input.L1:
		n.p.Prev()
		return true
	case input.R1:
		n.p.Next()
		return true
	}
	return false
}

func (n *NowPlaying) activate() {
	switch n.focus {
	case shuffleCtl:
		n.p.SetShuffle(!n.p.State().Shuffle)
	case prevCtl:
		n.p.Prev()
	case playCtl:
		n.p.Toggle()
	case nextCtl:
		n.p.Next()
	case repeatCtl:
		n.p.CycleRepeat()
	}
}

// Animating reports whether focus is moving. The app redraws the progress
// bar on its own 4-a-second tick while music plays.
func (n *NowPlaying) Animating(now time.Time) bool {
	for i := range n.values {
		if !n.values[i].Done(now) {
			return true
		}
	}
	return false
}

// Draw renders the frame.
func (n *NowPlaying) Draw(u *ui.Context) {
	s := n.p.State()
	widgets.DrawBackground(u, widgets.NowPlayingGround)
	status := u.Status
	u.Status.NowPlaying = "" // the screen itself shows what's playing
	widgets.StatusBar(u)
	u.Status = status

	top, bottom := float32(widgets.StatusHeight), float32(theme.ScreenH-widgets.HintsHeight)
	art := gfx.Rect{X: side, Y: top + (bottom-top-artSize)/2, W: artSize, H: artSize}
	shadow := art
	shadow.Y += 24
	u.Shape(raster.Shape{W: artSize, H: artSize, Radius: 16, Kind: raster.Shadow, GlowBlur: 60}, shadow, color.NRGBA{A: 128})
	widgets.Art(u, widgets.SpecFor(s.Track.Album), artSize, 16, art)

	x := art.X + artSize + colGap
	w := float32(theme.ScreenW) - side - x
	titleH, artistH, albumH, timeH := u.LineHeight(titleStyle), u.LineHeight(artistStyle), u.LineHeight(albumStyle), u.LineHeight(timeStyle)
	h := 32 + groupGap + titleH + artistH + albumH + groupGap + knob + 8 + timeH + groupGap + ctlRow
	y := top + (bottom-top-h)/2

	bw := widgets.Badge(u, media.Local, x, y, false)
	u.Text(formatLine(s), formatStyle, x+bw+12, u.CenterY(formatStyle, y, 32), theme.Ink3)
	y += 32 + groupGap
	u.Text(u.Fit(s.Track.Title, titleStyle, w), titleStyle, x, y, theme.Paper)
	y += titleH
	u.Text(u.Fit(s.Track.Artist, artistStyle, w), artistStyle, x, y, theme.Paper)
	y += artistH
	u.Text(u.Fit(s.Track.Album, albumStyle, w), albumStyle, x, y, theme.Ink2)
	y += albumH + groupGap
	drawProgress(u, s, x, y, w)
	y += knob + 8 + timeH + groupGap
	n.drawControls(u, s, x, y, w)

	widgets.HintBar(u, n.hints(s))
}

func formatLine(s player.State) string {
	if s.Rate == 0 {
		return s.Format
	}
	return fmt.Sprintf("%s · %g kHz", s.Format, float64(s.Rate)/1000)
}

// drawProgress draws the 6 px track, its accent fill, the 18 px knob, and the
// elapsed and total times below.
func drawProgress(u *ui.Context, s player.State, x, y, w float32) {
	var f float32
	if s.Duration > 0 {
		f = min(max(float32(s.Elapsed)/float32(s.Duration), 0), 1)
	}
	bar := gfx.Rect{X: x, Y: y + (knob-6)/2, W: w, H: 6}
	shape := raster.Shape{W: int(w), H: 6, Radius: 3, Kind: raster.Fill}
	u.Shape(shape, bar, theme.PaperA(0.18))
	if f > 0 {
		u.PushClip(gfx.Rect{X: x, Y: bar.Y, W: w * f, H: 6})
		u.Shape(shape, bar, u.Accent.Color)
		u.PopClip()
	}
	u.Shape(raster.Shape{W: knob, H: knob, Radius: knob / 2, Kind: raster.Fill},
		gfx.Rect{X: x + w*f - knob/2, Y: y, W: knob, H: knob}, theme.Paper)
	ty := y + knob + 8
	u.Text(clock(s.Elapsed), timeStyle, x, ty, theme.Ink2)
	total := clock(s.Duration)
	u.Text(total, timeStyle, x+w-u.TextWidth(total, timeStyle), ty, theme.Ink2)
}

func clock(d time.Duration) string {
	sec := int(d / time.Second)
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// drawControls spreads the five controls across the column, each focusable
// like a button.
func (n *NowPlaying) drawControls(u *ui.Context, s player.State, x, y, w float32) {
	cy := y + ctlRow/2
	for i := range controlIcons {
		cx := x + w/2 + controlOffset[i]
		d := float32(ctlFocus)
		if i == playCtl {
			d = ctlRow
		}
		k := float32(n.values[i].At(u.Now))
		box := gfx.Rect{X: cx - d/2, Y: cy - d/2, W: d, H: d}
		u.PushScale(cx, box.Y, 1+0.06*k)
		widgets.FocusGlow(u, box, float64(d)/2, k)
		widgets.FocusRing(u, box, float64(d)/2, k, theme.Ground)
		drawControl(u, s, i, cx, cy, box)
		u.Pop()
	}
}

func drawControl(u *ui.Context, s player.State, i int, cx, cy float32, box gfx.Rect) {
	switch i {
	case playCtl:
		u.Shape(raster.Shape{W: ctlRow, H: ctlRow, Radius: ctlRow / 2, Kind: raster.Ring, RingOffset: -2, RingWidth: 2}, box, u.Accent.Color)
		name := "play"
		if s.Playing {
			name = "pause"
		}
		u.Icon(name, 44, 2, true, cx-22, cy-22, theme.Paper)
	case prevCtl, nextCtl:
		u.Icon(controlIcons[i], 44, 2, true, cx-22, cy-22, theme.Paper)
	default:
		on := s.Shuffle
		if i == repeatCtl {
			on = s.Repeat != player.RepeatOff
		}
		col := theme.Ink3
		if on {
			col = u.Accent.Color
		}
		u.Icon(controlIcons[i], 34, 2, false, cx-17, cy-17, col)
		if on {
			u.Shape(raster.Shape{W: 6, H: 6, Radius: 3, Kind: raster.Fill}, gfx.Rect{X: cx - 3, Y: cy + 17 + 6, W: 6, H: 6}, col)
		}
		if i == repeatCtl && s.Repeat == player.RepeatOne {
			u.Text("1", oneStyle, cx+12, cy-26, col)
		}
	}
}

func (n *NowPlaying) hints(s player.State) []widgets.Hint {
	label := [controlCount]string{"Shuffle", "Previous", "Pause", "Next", "Repeat"}[n.focus]
	if n.focus == playCtl && !s.Playing {
		label = "Play"
	}
	return []widgets.Hint{{Key: "A", Label: label}, {Key: "B", Label: "Back"}, {Key: "L1/R1", Label: "Track"}}
}
