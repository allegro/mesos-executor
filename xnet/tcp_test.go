package xnet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allegro/mesos-executor/xnet/xnettest"
)

func TestIfTCPNetworkSenderReusesConnections(t *testing.T) {
	listener1, results1, err := xnettest.LoopbackServer("tcp")
	require.NoError(t, err)
	defer listener1.Close()
	listener2, results2, err := xnettest.LoopbackServer("tcp")
	require.NoError(t, err)
	defer listener2.Close()

	sender := &TCPSender{}
	defer sender.Release()

	_, err = sender.Send(Address(listener1.Addr().String()), []byte("test"))
	require.NoError(t, err)
	<-results1

	_, err = sender.Send(Address(listener1.Addr().String()), []byte("test"))
	require.NoError(t, err)
	<-results1

	_, err = sender.Send(Address(listener2.Addr().String()), []byte("test"))
	require.NoError(t, err)
	<-results2

	assert.Len(t, sender.connections, 2)
}

func TestIfTCPNetworkSenderReleasesResources(t *testing.T) {
	listener, _, err := xnettest.LoopbackServer("tcp")
	require.NoError(t, err)
	defer listener.Close()

	sender := &TCPSender{}
	_, err = sender.Send(Address(listener.Addr().String()), []byte("test"))
	require.NoError(t, err)
	sender.Release()

	assert.Empty(t, sender.connections)
}

func TestIfTCPNetworkSenderReturnsNumberOfSentBytes(t *testing.T) {
	listener, results, err := xnettest.LoopbackServer("tcp")
	require.NoError(t, err)
	defer listener.Close()

	sender := &TCPSender{}
	bytesSent, err := sender.Send(Address(listener.Addr().String()), []byte("test"))

	assert.NoError(t, err)
	assert.Equal(t, len([]byte("test")), bytesSent)
	assert.Equal(t, []byte("test"), <-results)
}

func TestIfTCPNetworkSenderReturnsErrorWhenConnectionUnavailable(t *testing.T) {
	sender := &TCPSender{}

	bytesSent, err := sender.Send("198.51.100.5", []byte("test")) // see RFC 5737 for more info about this IP address

	assert.Error(t, err)
	assert.Zero(t, bytesSent)
}
