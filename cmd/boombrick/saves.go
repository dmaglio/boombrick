package main

import (
	"sync"

	"github.com/ujjwal313/boombrick/internal/player"
)

// saves is the one way the queue is written while Boombrick has a screen.
// After the music is handed to the background player it writes nothing: a
// save then would put the background player's place back.
type saves struct {
	saver func(player.Snapshot) error

	mu         sync.Mutex
	handedOver bool // the background player owns the queue now
}

// save writes snap unless the music has been handed over.
func (s *saves) save(snap player.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handedOver {
		return nil
	}
	return s.saver(snap)
}

// now writes snap whatever the state: the hand-over's own save.
func (s *saves) now(snap player.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saver(snap)
}

// handOver runs f, a hand-over that saves with now. From before it starts,
// save writes nothing, once a save in flight has ended; when f fails, saves
// resume.
func (s *saves) handOver(f func() error) error {
	s.mu.Lock()
	s.handedOver = true
	s.mu.Unlock()
	err := f()
	if err != nil {
		s.mu.Lock()
		s.handedOver = false
		s.mu.Unlock()
	}
	return err
}
