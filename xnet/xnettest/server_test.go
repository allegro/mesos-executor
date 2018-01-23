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
	defer conn.Close()
	_, err = conn.Write([]byte("test"))

	require.NoError(t, err)
	assert.Equal(t, []byte("test"), <-results)
}

func TestIfPacketServerListensOnLoopbackAddress(t *testing.T) {
	conn, _, err := LoopbackPacketServer("udp")
	require.NoError(t, err)
	defer conn.Close()

	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	require.NoError(t, err)

	assert.True(t, addr.IP.IsLoopback())
}

func TestIfPacketServerSendsReceivedDataToChannel(t *testing.T) {
	conn, results, err := LoopbackPacketServer("udp")
	require.NoError(t, err)
	defer conn.Close()

	connIn, err := net.Dial("udp", conn.LocalAddr().String())
	require.NoError(t, err)
	defer connIn.Close()
	_, err = connIn.Write([]byte("test"))

	require.NoError(t, err)
	assert.Equal(t, []byte("test"), <-results)
}
