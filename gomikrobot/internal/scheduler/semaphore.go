package scheduler

// Semaphore is a channel-based counting semaphore for concurrency control.
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore creates a semaphore with the given capacity.
func NewSemaphore(cap int) *Semaphore {
	if cap <= 0 {
		cap = 1
	}
	return &Semaphore{ch: make(chan struct{}, cap)}
}

// TryAcquire attempts to acquire a slot without blocking.
// Returns true if a slot was acquired.
func (s *Semaphore) TryAcquire() bool {
	select {
	case s.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release frees a slot. Must only be called after a successful Acquire/TryAcquire.
func (s *Semaphore) Release() {
	<-s.ch
}

// Available returns the number of free slots.
func (s *Semaphore) Available() int {
	return cap(s.ch) - len(s.ch)
}

// Cap returns the total capacity.
func (s *Semaphore) Cap() int {
	return cap(s.ch)
}
