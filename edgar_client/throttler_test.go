package edgar_client

import (
  "testing"
  "time"

  "github.com/jmhodges/clock"
)

func checkThrottledTime(t* testing.T, clock clock.Clock, before time.Time, expected time.Duration) {
  actual := clock.Now().Sub(before)
  if actual != expected {
    t.Errorf("Mismatched duration, expected=%d(%s), actual=%d(%s)", expected, expected.String(), actual, actual.String())
  }
}

func TestWait(t *testing.T) {
  clock := clock.NewFake() 
  d := 10 * time.Millisecond
  count := 1
  throttler := newThrottler(clock, d, count)
  before := clock.Now()
  throttled := throttler.MaybeThrottle()
  if throttled {
    t.Errorf("Throttled the first call")
  }
  checkThrottledTime(t, clock, before, 0)

  before = clock.Now()
  throttled = throttler.MaybeThrottle()
  if !throttled {
    t.Errorf("Didn't throttle the second call")
  }
  checkThrottledTime(t, clock, before, d)

  before = clock.Now()
  throttled = throttler.MaybeThrottle()
  if !throttled {
    t.Errorf("Didn't throttle the third call")
  }
  checkThrottledTime(t, clock, before, d)
}

func TestWaitAfterForcedWait(t *testing.T) {
  clock := clock.NewFake() 
  d := 10 * time.Millisecond
  count := 1
  throttler := newThrottler(clock, d, count)
  before := clock.Now()
  throttled := throttler.MaybeThrottle()
  if throttled {
    t.Errorf("Throttled the first call")
  }
  checkThrottledTime(t, clock, before, 0)

  before = clock.Now()
  throttler.ForcedWait()
  checkThrottledTime(t, clock, before, d)

  before = clock.Now()
  throttled = throttler.MaybeThrottle()
  if throttled {
    t.Errorf("Throttled after a ForcedWait")
  }
  checkThrottledTime(t, clock, before, 0)

  before = clock.Now()
  throttled = throttler.MaybeThrottle()
  if !throttled {
    t.Errorf("Didn't throttle the second after a ForcedWait")
  }
  checkThrottledTime(t, clock, before, d)
}

func TestNoWaitAfterReset(t *testing.T) {
  clock := clock.NewFake() 
  d := 10 * time.Millisecond
  count := 1
  throttler := newThrottler(clock, d, count)
  before := clock.Now()
  throttled := throttler.MaybeThrottle()
  if throttled {
    t.Errorf("Throttled the first call")
  }
  checkThrottledTime(t, clock, before, 0)

  throttler.Reset()

  before = clock.Now()
  throttled = throttler.MaybeThrottle()
  if throttled {
    t.Errorf("Throttled after a Reset")
  }
  checkThrottledTime(t, clock, before, 0)

  before = clock.Now()
  throttled = throttler.MaybeThrottle()
  if !throttled {
    t.Errorf("Didn't throttle the second call after a Reset")
  }
  checkThrottledTime(t, clock, before, d)

  before = clock.Now()
  throttled = throttler.MaybeThrottle()
  if !throttled {
    t.Errorf("Didn't throttle the third call after a Reset")
  }
  checkThrottledTime(t, clock, before, d)

}
