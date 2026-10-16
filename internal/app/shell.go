package app

import (
	"time"

	"github.com/ujjwal313/boombrick/internal/platform/input"
	"github.com/ujjwal313/boombrick/internal/screens"
	"github.com/ujjwal313/boombrick/internal/ui"
	"github.com/ujjwal313/boombrick/internal/ui/anim"
	"github.com/ujjwal313/boombrick/internal/ui/widgets"
)

// Toast timing: shown for 2.5 s (docs/ui.md, "Toast"), fading in and out.
const (
	toastFor  = 2500 * time.Millisecond
	toastFade = 150 * time.Millisecond
)

// overlay is a full-screen screen opened over the tabs with a button.
type overlay struct {
	button input.Button
	screen screens.Screen
	can    func() bool // whether it may open now; nil means always
}

// shell routes input and drawing between each tab's stack of pages and the
// full-screen overlays, and runs their transitions and toasts. It knows
// nothing of SDL, so it is tested on its own.
type shell struct {
	stacks   [][]screens.Screen // per tab, root first
	tab      int
	trans    anim.Value     // the top page sliding in: 0 to 1 on a push, 1 to 0 on a pop
	leaving  screens.Screen // the page being popped, until its transition ends
	overlays []overlay
	open     int        // the open overlay, or -1
	showing  int        // the overlay drawn: the open one, or one fading out
	shown    anim.Value // the overlay's slide and fade, 0 to 1
	toast    string
	toastAt  time.Time
}

func newShell(roots ...screens.Screen) *shell {
	s := &shell{open: -1, showing: -1}
	for _, r := range roots {
		s.stacks = append(s.stacks, []screens.Screen{r})
	}
	s.trans.Set(1)
	return s
}

// top is the current tab's top page.
func (s *shell) top() screens.Screen {
	st := s.stacks[s.tab]
	return st[len(st)-1]
}

// active is the screen input goes to.
func (s *shell) active() screens.Screen {
	if s.open >= 0 {
		return s.overlays[s.open].screen
	}
	return s.top()
}

func (s *shell) push(p screens.Screen, now time.Time) {
	s.stacks[s.tab] = append(s.stacks[s.tab], p)
	s.leaving = nil
	s.trans.Set(0)
	s.trans.To(1, now, anim.Push, anim.EaseOutCubic)
}

func (s *shell) pop(now time.Time) {
	st := s.stacks[s.tab]
	if len(st) < 2 {
		return
	}
	s.leaving = st[len(st)-1]
	st[len(st)-1] = nil
	s.stacks[s.tab] = st[:len(st)-1]
	s.trans.To(0, now, anim.Push, anim.EaseOutCubic)
}

// showToast replaces any toast; one already fully shown stays at full alpha.
func (s *shell) showToast(msg string, now time.Time) {
	if s.toastAlpha(now) >= 1 {
		now = now.Add(-toastFade)
	}
	s.toast, s.toastAt = msg, now
}

// openOverlay opens overlay i. Over the tab it slides in; in place of another
// overlay it takes over as far as that one was shown, at once when fully, so
// the tab doesn't flash between them.
func (s *shell) openOverlay(i int, now time.Time) {
	if s.showing < 0 {
		s.shown.Set(0)
	}
	s.open, s.showing = i, i
	if o, ok := s.overlays[i].screen.(screens.Opener); ok {
		o.Opened()
	}
	s.shown.To(1, now, anim.Push, anim.EaseOutCubic)
}

func (s *shell) closeOverlay(now time.Time) {
	s.open = -1
	s.shown.To(0, now, anim.Push, anim.EaseOutCubic)
}

// handle routes a button event and reports whether the frame changed. An
// overlay's button opens it from anywhere, or closes it when it is open; B
// closes an open overlay, unless the overlay uses it, or pops a page; L1 and
// R1 switch tabs when no overlay is open.
func (s *shell) handle(e input.Event, now time.Time) bool {
	if e.Kind == input.Press {
		for i, o := range s.overlays {
			if e.Button != o.button {
				continue
			}
			switch {
			case s.open == i:
				s.closeOverlay(now)
			case o.can == nil || o.can():
				s.openOverlay(i, now)
			}
			return true
		}
		switch {
		case e.Button == input.B && s.open >= 0:
			if b, ok := s.overlays[s.open].screen.(screens.Backer); ok && b.Back() {
				return true
			}
			s.closeOverlay(now)
			return true
		case e.Button == input.B && len(s.stacks[s.tab]) > 1:
			s.pop(now)
			return true
		case (e.Button == input.L1 || e.Button == input.R1) && s.open < 0:
			t := s.tab - 1
			if e.Button == input.R1 {
				t = s.tab + 1
			}
			if t < 0 || t >= len(s.stacks) {
				return false
			}
			s.tab, s.leaving = t, nil
			s.trans.Set(1)
			return true
		}
	}
	return s.active().Handle(e, now)
}

// draw renders the tab's pages, the overlay sliding 40 px and fading over
// them, then any toast.
func (s *shell) draw(u *ui.Context) {
	now := u.Now
	t := float32(s.shown.At(now))
	if s.showing < 0 || t < 1 {
		s.drawTab(u)
	}
	if s.showing >= 0 && t > 0 {
		slide(u, s.overlays[s.showing].screen, t)
	}
	if s.showing >= 0 && s.open < 0 && s.shown.Done(now) {
		s.showing = -1
	}
	if a := s.toastAlpha(now); a > 0 {
		widgets.Toast(u, s.toast, a)
	}
}

func (s *shell) drawTab(u *ui.Context) {
	st := s.stacks[s.tab]
	top := st[len(st)-1]
	if s.leaving != nil && s.trans.Done(u.Now) {
		s.leaving = nil
		s.trans.Set(1)
	}
	t := float32(s.trans.At(u.Now))
	switch {
	case s.leaving != nil: // popping: the page fades out over the one below
		top.Draw(u)
		slide(u, s.leaving, t)
	case t < 1 && len(st) > 1: // pushing: the new page fades in over the one below
		st[len(st)-2].Draw(u)
		slide(u, top, t)
	default:
		top.Draw(u)
	}
}

// slide draws scr faded to t and shifted 40*(1-t) px right.
func slide(u *ui.Context, scr screens.Screen, t float32) {
	u.PushTranslate(40*(1-t), 0)
	u.Alpha = t
	scr.Draw(u)
	u.Alpha = 1
	u.Pop()
}

func (s *shell) toastAlpha(now time.Time) float32 {
	d := now.Sub(s.toastAt)
	if s.toast == "" || d < 0 || d >= toastFor {
		return 0
	}
	return float32(min(d, toastFor-d, toastFade)) / float32(toastFade)
}

// animating reports whether the next frame differs without input.
func (s *shell) animating(now time.Time) bool {
	if !s.trans.Done(now) || !s.shown.Done(now) {
		return true
	}
	if d := now.Sub(s.toastAt); s.toast != "" && d >= 0 && (d < toastFade || d >= toastFor-toastFade && d < toastFor) {
		return true
	}
	return s.active().Animating(now)
}

// wake is when the loop must next draw though nothing animates now: a toast
// starting to fade out.
func (s *shell) wake(now time.Time) (time.Time, bool) {
	out := s.toastAt.Add(toastFor - toastFade)
	if s.toast != "" && now.Before(out) {
		return out, true
	}
	return time.Time{}, false
}
