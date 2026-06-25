package circuitbreaker

import (
	"sync"
	"testing"
	"time"
)

func TestNewBreakerStartsClosed(t *testing.T) {
	cfg := Config{FailureThreshold: 5, ResetTimeout: 30 * time.Second}
	b := New(cfg)
	if b.State() != StateClosed {
		t.Fatalf("expected closed, got %s", b.State())
	}
}

func TestAllowReturnsTrueWhenClosed(t *testing.T) {
	b := New(Config{FailureThreshold: 5, ResetTimeout: time.Second})
	if !b.Allow() {
		t.Fatal("expected allow when closed")
	}
}

func TestFailureOpensAfterThreshold(t *testing.T) {
	b := New(Config{FailureThreshold: 3, ResetTimeout: time.Minute})
	for i := 0; i < 3; i++ {
		b.Failure()
	}
	if b.State() != StateOpen {
		t.Fatalf("expected open, got %s", b.State())
	}
}

func TestAllowReturnsFalseWhenOpen(t *testing.T) {
	b := New(Config{FailureThreshold: 1, ResetTimeout: time.Minute})
	b.Failure()
	if b.Allow() {
		t.Fatal("expected denied when open")
	}
}

func TestHalfOpenAfterTimeout(t *testing.T) {
	b := New(Config{FailureThreshold: 1, ResetTimeout: 50 * time.Millisecond})
	b.Failure()
	time.Sleep(100 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("expected allow after timeout transitions to half-open")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("expected half-open, got %s", b.State())
	}
}

func TestSuccessClosesFromHalfOpen(t *testing.T) {
	b := New(Config{FailureThreshold: 1, ResetTimeout: 50 * time.Millisecond})
	b.Failure()
	time.Sleep(100 * time.Millisecond)
	b.Allow() // transitions to half-open
	b.Success()
	if b.State() != StateClosed {
		t.Fatalf("expected closed, got %s", b.State())
	}
}

func TestFailureReopensFromHalfOpen(t *testing.T) {
	b := New(Config{FailureThreshold: 1, ResetTimeout: 50 * time.Millisecond})
	b.Failure()
	time.Sleep(100 * time.Millisecond)
	b.Allow() // transitions to half-open
	b.Failure()
	if b.State() != StateOpen {
		t.Fatalf("expected open, got %s", b.State())
	}
}

func TestConcurrentAccess(t *testing.T) {
	b := New(Config{FailureThreshold: 10, ResetTimeout: time.Minute})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Allow()
			b.Success()
			b.Failure()
		}()
	}
	wg.Wait()
}

func TestEndpointBreakerIsolation(t *testing.T) {
	eb := NewEndpointBreaker(Config{FailureThreshold: 2, ResetTimeout: time.Minute})
	b1 := eb.Get("ep-1")
	b2 := eb.Get("ep-2")

	b1.Failure()
	b1.Failure()

	if b1.State() != StateOpen {
		t.Fatalf("ep-1 should be open, got %s", b1.State())
	}
	if b2.State() != StateClosed {
		t.Fatalf("ep-2 should be closed, got %s", b2.State())
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	b := New(Config{FailureThreshold: 3, ResetTimeout: time.Minute})
	b.Failure()
	b.Failure()
	b.Success()
	b.Failure()
	if b.State() != StateClosed {
		t.Fatalf("expected closed after success resets count, got %s", b.State())
	}
}
