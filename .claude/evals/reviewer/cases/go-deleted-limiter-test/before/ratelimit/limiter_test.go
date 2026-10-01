package ratelimit

import (
	"testing"
	"time"
)

func TestAllowStopsAtMax(t *testing.T) {
	l := New(3, time.Minute)
	now := time.Unix(1000, 0)
	for i := 0; i < 3; i++ {
		if !l.Allow("k", now) {
			t.Fatalf("call %d refused, want allowed", i+1)
		}
	}
	if l.Allow("k", now) {
		t.Fatal("4th call allowed, want refused")
	}
}

func TestAllowForgetsOldCalls(t *testing.T) {
	l := New(1, time.Minute)
	l.Allow("k", time.Unix(1000, 0))
	if !l.Allow("k", time.Unix(1061, 0)) {
		t.Fatal("call after the window refused")
	}
}
