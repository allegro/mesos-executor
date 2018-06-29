package xnet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allegro/mesos-executor/xnet/xnettest"
)

func TestUDPNetworkSendShouldReturnErrorWhenConnectionUnavailable(t *testing.T) {
	sender := &UDPSender{}

	bytesSent, err := sender.Send("198.51.100.5", [][]byte{[]byte("test")}) // see RFC 5737 for more info about this IP address

	assert.Error(t, err)
	assert.Zero(t, bytesSent)
}

func TestUDPNetworkSendShouldReturnNumberOfSentBytes(t *testing.T) {
	conn, result, err := xnettest.LoopbackPacketServer("udp")
	require.NoError(t, err)
	defer conn.Close()

	sender := &UDPSender{}

	bytesSent, err := sender.Send(Address(conn.LocalAddr().String()), [][]byte{[]byte("test")})

	require.NoError(t, err)
	assert.Equal(t, 4, bytesSent)
	assert.Equal(t, []byte("test"), <-result)
}
