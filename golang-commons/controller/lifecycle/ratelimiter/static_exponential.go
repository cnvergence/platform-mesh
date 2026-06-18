package ratelimiter

import (
	"sync"
	"time"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
)

type StaticThenExponentialRateLimiter[T comparable] struct {
	failuresLock   sync.RWMutex
	staticAttempts map[T]time.Time

	staticDelay  time.Duration
	staticWindow time.Duration

	exponential workqueue.TypedRateLimiter[T]
	clock       clock.Clock
}

func NewStaticThenExponentialRateLimiter[T comparable](cfg Config) (*StaticThenExponentialRateLimiter[T], error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &StaticThenExponentialRateLimiter[T]{
		staticDelay:  cfg.StaticRequeueDelay,
		staticWindow: cfg.StaticWindow,
		exponential: workqueue.NewTypedItemExponentialFailureRateLimiter[T](
			cfg.ExponentialInitialBackoff,
			cfg.ExponentialMaxBackoff,
		),
		staticAttempts: make(map[T]time.Time),
		clock:          clock.RealClock{},
	}, nil
}

func (r *StaticThenExponentialRateLimiter[T]) When(item T) time.Duration {
	now := r.clock.Now()

	r.failuresLock.RLock()
	first, exists := r.staticAttempts[item]
	r.failuresLock.RUnlock()
	if !exists {
		r.failuresLock.Lock()
		r.staticAttempts[item] = now
		r.failuresLock.Unlock()
		return r.staticDelay
	}

	timeSinceFirst := now.Sub(first)
	if timeSinceFirst <= r.staticWindow {
		return r.staticDelay
	}

	return r.exponential.When(item)
}

func (r *StaticThenExponentialRateLimiter[T]) Forget(item T) {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	delete(r.staticAttempts, item)
	r.exponential.Forget(item)
}

func (r *StaticThenExponentialRateLimiter[T]) NumRequeues(item T) int {
	return r.exponential.NumRequeues(item)
}
