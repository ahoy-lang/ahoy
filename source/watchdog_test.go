package main

import (
	"runtime"
	"testing"
	"time"
)

// The heap watchdog (watchHeapMB in main.go) must fire while over-cap heap
// is held, and stay silent when the heap is under cap. An 8MB live blob is
// far above the 1MB test cap and far below any dangerous size.
func TestWatchHeapFiresWhenOverCap(t *testing.T) {
	fired := make(chan float64, 8)
	stop := watchHeapMB(1, 5*time.Millisecond, func(mb float64) {
		select {
		case fired <- mb:
		default:
		}
	})
	defer stop()
	blob := make([]byte, 8<<20)
	select {
	case mb := <-fired:
		if mb < 1 {
			t.Fatalf("watchdog fired with %.2f MB, want >= 1", mb)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watchdog did not fire while 8MB over a 1MB cap")
	}
	runtime.KeepAlive(blob)
}

func TestWatchHeapSilentWhenUnderCap(t *testing.T) {
	fired := make(chan float64, 8)
	stop := watchHeapMB(1<<30, 5*time.Millisecond, func(mb float64) {
		select {
		case fired <- mb:
		default:
		}
	})
	defer stop()
	blob := make([]byte, 8<<20)
	select {
	case mb := <-fired:
		t.Fatalf("watchdog fired with %.2f MB under a 1TB cap", mb)
	case <-time.After(60 * time.Millisecond):
	}
	runtime.KeepAlive(blob)
}
