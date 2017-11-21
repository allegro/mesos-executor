package executor

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIfRandomDurationReturnsDurationInRange(t *testing.T) {
	r := newRandom()
	assert.Zero(t, r.Duration(time.Nanosecond))
}

func TestIfRandomDurationReturnsDurationInRangeWhenStaticSeed(t *testing.T) {
	r := &mathRandom{*rand.New(rand.NewSource(0))}
	assert.Equal(t, "526.058514ms", r.Duration(time.Second).String())
}
