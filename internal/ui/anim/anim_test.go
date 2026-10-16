package anim

import (
	"testing"
	"time"
)

var t0 = time.Unix(0, 0)

func TestValueAnimates(t *testing.T) {
	var v Value
	v.To(10, t0, 100*time.Millisecond, Linear)
	if got := v.At(t0.Add(50 * time.Millisecond)); got != 5 {
		t.Errorf("halfway = %v, want 5", got)
	}
	if v.Done(t0.Add(99 * time.Millisecond)) {
		t.Error("done too early")
	}
	if !v.Done(t0.Add(100*time.Millisecond)) || v.At(t0.Add(time.Second)) != 10 {
		t.Error("should finish at the target")
	}
}

func TestRetargetStartsFromCurrentValue(t *testing.T) {
	var v Value
	v.To(10, t0, 100*time.Millisecond, Linear)
	v.To(0, t0.Add(50*time.Millisecond), 100*time.Millisecond, Linear)
	if got := v.At(t0.Add(50 * time.Millisecond)); got != 5 {
		t.Errorf("after retarget = %v, want 5", got)
	}
	if got := v.At(t0.Add(100 * time.Millisecond)); got != 2.5 {
		t.Errorf("halfway back = %v, want 2.5", got)
	}
}

func TestSetJumps(t *testing.T) {
	var v Value
	v.Set(3)
	if v.At(t0) != 3 || !v.Done(t0) || v.Target() != 3 {
		t.Errorf("Set: At=%v Done=%v Target=%v", v.At(t0), v.Done(t0), v.Target())
	}
}

func TestEaseOutCubic(t *testing.T) {
	for in, want := range map[float64]float64{0: 0, 0.5: 0.875, 1: 1} {
		if got := EaseOutCubic(in); got != want {
			t.Errorf("EaseOutCubic(%v) = %v, want %v", in, got, want)
		}
	}
}
