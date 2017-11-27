package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIfNotFailsToGetCPUTime(t *testing.T) {
	_, err := CPUTime()

	assert.NoError(t, err)
}
