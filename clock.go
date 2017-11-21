package executor

import (
	"math/rand"
	"time"
)

type clock interface {
	// Until returns the duration until t.
	Until(t time.Time) time.Duration
}

type random interface {
	// Duration returns, as an Duration, a non-negative pseudo-random number in [0,max).
	Duration(max time.Duration) time.Duration
}

type systemClock struct{}

func (c systemClock) Until(t time.Time) time.Duration {
	return time.Until(t)
}

type mathRandom struct {
	rand.Rand
}

func newRandom() *mathRandom {
	return &mathRandom{*rand.New(rand.NewSource(time.Now().Unix()))}
}

func (r mathRandom) Duration(max time.Duration) time.Duration {
	return time.Duration(r.Intn(int(max)))
}
