package focus

import "testing"

func TestNextInGrid(t *testing.T) {
	// Three 212 px album cards per row, as in the Library grid.
	var items []Rect
	for row := 0; row < 2; row++ {
		for col := 0; col < 3; col++ {
			items = append(items, Rect{X: float64(col) * 248, Y: float64(row) * 300, W: 212, H: 212})
		}
	}
	cases := []struct {
		from int
		d    Dir
		want int
	}{
		{0, Right, 1}, {1, Right, 2}, {2, Right, 2}, {0, Down, 3}, {4, Up, 1}, {3, Left, 3}, {5, Left, 4},
	}
	for _, tc := range cases {
		if got := Next(items, tc.from, tc.d); got != tc.want {
			t.Errorf("Next(%d, %v) = %d, want %d", tc.from, tc.d, got, tc.want)
		}
	}
}

func TestNextPrefersTheAlignedItem(t *testing.T) {
	// Four 200 px cards above five 166 px tiles, as on Home.
	var items []Rect
	for i := 0; i < 4; i++ {
		items = append(items, Rect{X: float64(i) * 232, Y: 0, W: 200, H: 277})
	}
	for i := 0; i < 5; i++ {
		items = append(items, Rect{X: float64(i) * 194, Y: 330, W: 166, H: 166})
	}
	// Card 1 is centred at x=332; tile 1 (centre 277) is closer than tile 2 (centre 471).
	if got := Next(items, 1, Down); got != 5 {
		t.Errorf("Next(card 1, Down) = %d, want 5", got)
	}
}

func TestScroll(t *testing.T) {
	cases := []struct{ offset, top, bottom, view, want float64 }{
		{0, 100, 200, 300, 0},     // already visible
		{0, 250, 400, 300, 100},   // below: scroll down just enough
		{200, 100, 150, 300, 100}, // above: scroll up to its top
	}
	for _, tc := range cases {
		if got := Scroll(tc.offset, tc.top, tc.bottom, tc.view); got != tc.want {
			t.Errorf("Scroll(%v) = %v, want %v", tc, got, tc.want)
		}
	}
}
