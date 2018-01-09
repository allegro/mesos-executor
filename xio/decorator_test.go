package xio

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIfLimitsWritesBySize(t *testing.T) {
	writer := DecorateWriter(os.Stdout, SizeLimit(1))

	n, err := writer.Write([]byte("too big"))

	assert.Equal(t, 0, n)
	assert.Error(t, err)
	assert.Equal(t, err, ErrSizeLimitExceeded)
}

func TestIfLimitsWritesByRate(t *testing.T) {
	writer := DecorateWriter(os.Stdout, RateLimit(1))

	n, err := writer.Write([]byte("1"))
	require.Equal(t, 1, n)
	require.NoError(t, err)

	n, err = writer.Write([]byte("2"))

	assert.Equal(t, 0, n)
	assert.Error(t, err)
	assert.Equal(t, ErrRateLimitExceeded, err)
}
