package mesosutils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDuration(t *testing.T) {
	assert.Equal(t, time.Duration(time.Second*3/2), Duration(1.5))
}
