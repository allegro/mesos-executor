package xnettest

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIfServerListensOnLoopbackAddress(t *testing.T) {
	listener, _, err := LoopbackServer("tcp")
	require.NoError(t, err)
	defer listener.Close()

	addr, err := net.ResolveTCPAddr("tcp", listener.Addr().String())
	require.NoError(t, err)

	assert.True(t, addr.IP.IsLoopback())
}

func TestIfServerSendsReceivedDataToChannel(t *testing.T) {
	listener, results, err := LoopbackServer("tcp")
	require.NoError(t, err)
	defer listener.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	conn.Write([]byte("test"))

	assert.Equal(t, []byte("test"), <-results)
}
