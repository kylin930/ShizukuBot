package core

import (
	"testing"
	"time"
)

func TestSlidingWindowIdleAndFullRevolution(t *testing.T) {
	now := time.Unix(1800000000, 0)
	w := NewSlidingWindow(60, time.Second)
	w.now = func() time.Time { return now }
	w.Add(120)
	if w.GetQPS() != 2 {
		t.Fatal(w.GetQPS())
	}
	now = now.Add(60 * time.Second)
	if w.GetQPS() != 0 {
		t.Fatal("idle QPS did not expire")
	}
	w.Add(60)
	now = now.Add(121 * time.Second)
	w.Add(60)
	if w.GetQPS() != 1 {
		t.Fatal("full revolution retained expired buckets")
	}
	now = now.Add(-time.Hour)
	if w.GetQPS() != 0 {
		t.Fatal("clock rollback")
	}
}
