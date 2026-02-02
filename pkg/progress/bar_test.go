package progress

import (
	"testing"
	"time"
)

func TestBar(t *testing.T) {
	bar := NewBar()
	bar.Init(100)

	// Test Add
	bar.Add(10)
	if bar.GetBytes() != 10 {
		t.Errorf("expected 10 bytes, got %d", bar.GetBytes())
	}

	bar.Add(20)
	if bar.GetBytes() != 30 {
		t.Errorf("expected 30 bytes, got %d", bar.GetBytes())
	}

	// Test Complete
	bar.Complete()

	// Test Close
	if err := bar.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestBarUnknownTotal(t *testing.T) {
	bar := NewBar()
	bar.Init(0) // Unknown total

	bar.Add(100)
	bar.Add(200)

	if bar.GetBytes() != 300 {
		t.Errorf("expected 300 bytes, got %d", bar.GetBytes())
	}

	bar.Close()
}

func TestSilent(t *testing.T) {
	silent := NewSilent()

	// Should not panic
	silent.Init(100)
	silent.Add(10)
	silent.Add(20)
	silent.Complete()

	if err := silent.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestBarGetElapsed(t *testing.T) {
	bar := NewBar()
	bar.Init(100)

	time.Sleep(100 * time.Millisecond)
	elapsed := bar.GetElapsed()
	if elapsed < 0.05 {
		t.Errorf("expected at least 0.05s elapsed, got %f", elapsed)
	}

	bar.Close()
}
