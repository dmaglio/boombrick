package screens

// AddedToQueue is the toast shown after something is added to the queue.
const AddedToQueue = "Added to queue"

// Scanning is the toast shown when a rescan is asked for.
const Scanning = "Scanning the SD card"

// Nav lets a screen open and close pages on its tab and show toasts.
type Nav interface {
	// Push opens s on top of the current tab.
	Push(s Screen)
	// Pop closes the current tab's top page.
	Pop()
	// Toast shows msg above the hint bar for a moment.
	Toast(msg string)
}

// Backer is a screen that sometimes uses B itself, as the Queue does to
// cancel a move. Back reports whether it did; if not, B goes back as usual.
type Backer interface {
	Back() bool
}

// Opener is a screen that wants to know each time its overlay opens, to drop
// state left from the last time it was shown.
type Opener interface {
	Opened()
}
