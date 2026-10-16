// Package focus moves focus between on-screen items with the D-pad and keeps
// the focused item in view.
package focus

import "math"

// Rect is an item's bounds.
type Rect struct{ X, Y, W, H float64 }

// Dir is a D-pad direction.
type Dir uint8

const (
	Up Dir = iota
	Down
	Left
	Right
)

// Next returns the index to focus when moving from items[cur] in direction d,
// or cur if nothing lies that way. Candidates must start beyond the current
// item's edge; the nearest wins, with sideways offset counting double so focus
// prefers to move in a straight line.
func Next(items []Rect, cur int, d Dir) int {
	if cur < 0 || cur >= len(items) {
		return cur
	}
	c := items[cur]
	best, bestScore := cur, math.Inf(1)
	for i, it := range items {
		if i == cur {
			continue
		}
		var gap, off float64
		switch d {
		case Right:
			gap, off = it.X-(c.X+c.W), (it.Y+it.H/2)-(c.Y+c.H/2)
		case Left:
			gap, off = c.X-(it.X+it.W), (it.Y+it.H/2)-(c.Y+c.H/2)
		case Down:
			gap, off = it.Y-(c.Y+c.H), (it.X+it.W/2)-(c.X+c.W/2)
		case Up:
			gap, off = c.Y-(it.Y+it.H), (it.X+it.W/2)-(c.X+c.W/2)
		}
		if gap < -1 {
			continue
		}
		if score := math.Max(gap, 0) + 2*math.Abs(off); score < bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

// Scroll returns the offset that keeps [top, bottom) inside a viewport of
// height view, moving as little as possible from offset.
func Scroll(offset, top, bottom, view float64) float64 {
	if top < offset {
		return top
	}
	if bottom > offset+view {
		return bottom - view
	}
	return offset
}
