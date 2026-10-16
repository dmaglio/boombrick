package input

// brickButtons is the Brick's joystick layout, from NextUI's tg5040
// platform.h.
var brickButtons = map[uint8]Button{
	0: B, 1: A, 2: Y, 3: X,
	4: L1, 5: R1,
	6: Select, 7: Start, 8: Menu,
	13: VolDown, 14: VolUp,
}

const (
	brickAxisL2      = 2
	brickAxisR2      = 5
	triggerThreshold = 16384
	scancodePower    = 102 // SDL_SCANCODE_POWER
)

// SDL hat bits.
const (
	hatUp    = 1
	hatRight = 2
	hatDown  = 4
	hatLeft  = 8
)

// desktopKeys maps SDL scancodes for development on a PC; README.md lists them.
var desktopKeys = map[int]Button{
	82: Up, 81: Down, 80: Left, 79: Right,
	14: A, 13: B, 12: X, 24: Y, // K J I U
	20: L1, 8: R1, // Q E
	30: L2, 32: R2, // 1 3
	40: Start, 43: Select, 41: Menu, // Return Tab Escape
	19: Power, 45: VolDown, 46: VolUp, // P - =
	scancodePower: Power,
}

// Mapper turns device events into button events. It remembers hat and trigger
// state so each physical change produces exactly one Press or Release.
type Mapper struct {
	hat    uint8
	l2, r2 bool
}

// JoyButton maps a joystick button.
func (m *Mapper) JoyButton(button uint8, down bool) []Event {
	b, ok := brickButtons[button]
	if !ok {
		return nil
	}
	return []Event{{Button: b, Kind: kind(down)}}
}

// JoyHat maps the D-pad hat's new value.
func (m *Mapper) JoyHat(value uint8) []Event {
	var out []Event
	for _, h := range [...]struct {
		bit uint8
		b   Button
	}{{hatUp, Up}, {hatRight, Right}, {hatDown, Down}, {hatLeft, Left}} {
		was, is := m.hat&h.bit != 0, value&h.bit != 0
		if was != is {
			out = append(out, Event{Button: h.b, Kind: kind(is)})
		}
	}
	m.hat = value
	return out
}

// JoyAxis maps the analogue L2 and R2 triggers to button presses.
func (m *Mapper) JoyAxis(axis uint8, value int16) []Event {
	var state *bool
	var b Button
	switch axis {
	case brickAxisL2:
		state, b = &m.l2, L2
	case brickAxisR2:
		state, b = &m.r2, R2
	default:
		return nil
	}
	down := int(value) > triggerThreshold
	if down == *state {
		return nil
	}
	*state = down
	return []Event{{Button: b, Kind: kind(down)}}
}

// Key maps a keyboard scancode. OS key repeat is ignored because Repeater
// produces repeats for every input device.
func (m *Mapper) Key(scancode int, down, osRepeat bool) []Event {
	if osRepeat {
		return nil
	}
	b, ok := desktopKeys[scancode]
	if !ok {
		return nil
	}
	return []Event{{Button: b, Kind: kind(down)}}
}

func kind(down bool) Kind {
	if down {
		return Press
	}
	return Release
}
