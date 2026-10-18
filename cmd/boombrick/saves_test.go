package main

import (
	"errors"
	"testing"

	"github.com/ujjwal313/boombrick/internal/player"
)

func TestNoSavesAfterAHandOver(t *testing.T) {
	var saved int
	g := &saves{saver: func(player.Snapshot) error { saved++; return nil }}
	if err := g.save(player.Snapshot{}); err != nil || saved != 1 {
		t.Fatalf("before the hand-over: saved %d, %v", saved, err)
	}
	err := g.handOver(func() error {
		if err := g.save(player.Snapshot{}); err != nil || saved != 1 {
			t.Errorf("during the hand-over: saved %d, %v", saved, err)
		}
		return g.now(player.Snapshot{})
	})
	if err != nil || saved != 2 {
		t.Fatalf("hand-over = %v, saved %d; want its own save only", err, saved)
	}
	if err := g.save(player.Snapshot{}); err != nil || saved != 2 {
		t.Fatalf("after the hand-over: saved %d, %v", saved, err)
	}
}

func TestSavesResumeAfterAFailedHandOver(t *testing.T) {
	var saved int
	g := &saves{saver: func(player.Snapshot) error { saved++; return nil }}
	failed := errors.New("no player")
	if err := g.handOver(func() error { return failed }); err != failed {
		t.Fatalf("hand-over = %v", err)
	}
	if err := g.save(player.Snapshot{}); err != nil || saved != 1 {
		t.Fatalf("after a failed hand-over: saved %d, %v", saved, err)
	}
}
