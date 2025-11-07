package edgar_client

import (
  "time"

  "github.com/jmhodges/clock"
)

type Throttler struct {
  clock clock.Clock
  d time.Duration
  originalCount int
  remaining int
}

func (t *Throttler) MaybeThrottle() bool {
  // We let the first call go through.
  t.remaining -= 1
  if t.remaining < 0 {
    t.clock.Sleep(t.d)
    // The -1 is to carry over the call.
    t.remaining = t.originalCount - 1
    return true
  }
  return false
}

func (t *Throttler) ForcedWait() {
  t.clock.Sleep(t.d)
  t.remaining = t.originalCount
}

func (t *Throttler) RemainingFetches() int {
  return t.remaining
}

func (t *Throttler) Reset() {
  t.remaining = t.originalCount
}

func newThrottler(clock clock.Clock, d time.Duration, fetches int) *Throttler {
  return &Throttler{clock, d, fetches, fetches}
}
