package circuitbreaker

import (
	"sync"
	"time"
)

type State int

const (
	StateClosed   State = 0
	StateOpen     State = 1
	StateHalfOpen State = 2
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type Breaker struct {
	failureThreshold int
	resetTimeout     time.Duration
	halfOpenMaxReqs  int

	mu              sync.RWMutex
	state           State
	failureCount    int
	lastFailureTime time.Time
	halfOpenReqs    int
}

type Config struct {
	FailureThreshold int
	ResetTimeout     time.Duration
	HalfOpenMaxReqs  int
}

func New(cfg Config) *Breaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMaxReqs <= 0 {
		cfg.HalfOpenMaxReqs = 1
	}
	return &Breaker{
		failureThreshold: cfg.FailureThreshold,
		resetTimeout:     cfg.ResetTimeout,
		halfOpenMaxReqs:  cfg.HalfOpenMaxReqs,
		state:            StateClosed,
	}
}

func (b *Breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(b.lastFailureTime) > b.resetTimeout {
			b.state = StateHalfOpen
			b.halfOpenReqs = 0
			return true
		}
		return false
	case StateHalfOpen:
		if b.halfOpenReqs < b.halfOpenMaxReqs {
			b.halfOpenReqs++
			return true
		}
		return false
	}
	return true
}

func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateHalfOpen:
		b.state = StateClosed
		b.failureCount = 0
		b.halfOpenReqs = 0
	case StateClosed:
		b.failureCount = 0
	}
}

func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failureCount++
	b.lastFailureTime = time.Now()

	switch b.state {
	case StateHalfOpen:
		b.state = StateOpen
	case StateClosed:
		if b.failureCount >= b.failureThreshold {
			b.state = StateOpen
		}
	}
}

type EndpointBreaker struct {
	mu       sync.RWMutex
	breakers map[string]*Breaker
	config   Config
}

func NewEndpointBreaker(cfg Config) *EndpointBreaker {
	return &EndpointBreaker{
		breakers: make(map[string]*Breaker),
		config:   cfg,
	}
}

func (eb *EndpointBreaker) Get(endpointID string) *Breaker {
	eb.mu.RLock()
	b, ok := eb.breakers[endpointID]
	eb.mu.RUnlock()
	if ok {
		return b
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()
	if b, ok = eb.breakers[endpointID]; ok {
		return b
	}
	b = New(eb.config)
	eb.breakers[endpointID] = b
	return b
}

func (eb *EndpointBreaker) Reset(endpointID string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.breakers, endpointID)
}
