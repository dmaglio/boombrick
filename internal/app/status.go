package app

import (
	"time"

	"github.com/ujjwal313/boombrick/internal/platform"
)

// statusEvery is how often the battery and network are read.
const statusEvery = 10 * time.Second

// watchStatus calls read at once and then every interval until stop is
// closed, and hands each reading that differs from the one before to out,
// then calls wake. out holds one reading; one the loop hasn't taken yet is
// replaced, so it only ever sees the latest.
func watchStatus(read func() platform.Status, every time.Duration, out chan platform.Status, wake func(), stop <-chan struct{}) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	var last platform.Status
	for first := true; ; first = false {
		if st := read(); first || st != last {
			last = st
			select {
			case <-out:
			default:
			}
			out <- st // the only sender, so there is room now
			wake()
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}
