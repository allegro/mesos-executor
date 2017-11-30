package appender

import (
	"bufio"
	"net"
	"testing"

	"github.com/allegro/mesos-executor/servicelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIfSendsLogsToLogstash(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer ln.Close()
		conn, _ := ln.Accept()
		reader := bufio.NewReader(conn)
		bytes, _, err := reader.ReadLine()

		require.NoError(t, err)
		assert.Contains(t, string(bytes), "\"@version\":\"1\"")
		assert.Contains(t, string(bytes), "@timestamp")

		done <- struct{}{}
	}()

	entries := make(chan servicelog.Entry)
	logstash, err := NewLogstash("tcp", ln.Addr().String())
	require.NoError(t, err)

	go logstash.Append(entries)

	entries <- servicelog.Entry{}
	<-done
}
