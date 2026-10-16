package app

import (
	"sync"
	"testing"
	"time"

	"github.com/ujjwal313/boombrick/internal/platform"
)

func TestWatchStatus(t *testing.T) {
	var mu sync.Mutex
	reading := platform.Status{Battery: 80}
	reads := 0
	read := func() platform.Status {
		mu.Lock()
		defer mu.Unlock()
		reads++
		return reading
	}
	set := func(st platform.Status) {
		mu.Lock()
		defer mu.Unlock()
		reading = st
	}
	out := make(chan platform.Status, 1)
	wakes := make(chan struct{}, 16)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchStatus(read, 5*time.Millisecond, out, func() { wakes <- struct{}{} }, stop)
	}()

	next := func(what string) platform.Status {
		t.Helper()
		select {
		case <-wakes:
		case <-time.After(2 * time.Second):
			t.Fatalf("no wake-up for %s", what)
		}
		return <-out
	}
	if got := next("the first reading"); got != (platform.Status{Battery: 80}) {
		t.Fatalf("first reading %+v", got)
	}
	// Unchanged readings aren't handed over.
	for {
		mu.Lock()
		n := reads
		mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case st := <-out:
		t.Fatalf("an unchanged reading was handed over: %+v", st)
	default:
	}
	set(platform.Status{Battery: 79, Online: true})
	if got := next("a change"); got != (platform.Status{Battery: 79, Online: true}) {
		t.Fatalf("after a change %+v", got)
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the watcher didn't stop")
	}
}

// A reading the loop hasn't taken is replaced by the next, not queued behind
// it, and the watcher never blocks on it.
func TestWatchStatusKeepsOnlyTheLatest(t *testing.T) {
	var mu sync.Mutex
	battery := 50
	read := func() platform.Status {
		mu.Lock()
		defer mu.Unlock()
		battery--
		return platform.Status{Battery: battery}
	}
	out := make(chan platform.Status, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchStatus(read, time.Millisecond, out, func() {}, stop)
	}()
	time.Sleep(30 * time.Millisecond)
	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the watcher blocked with a reading waiting")
	}
	mu.Lock()
	last := battery
	mu.Unlock()
	if got := <-out; got.Battery != last {
		t.Fatalf("waiting reading %d, want the latest, %d", got.Battery, last)
	}
}
