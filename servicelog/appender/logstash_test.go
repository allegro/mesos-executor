package appender

import (
	"bufio"
	"net"
	"os"
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
		assert.Contains(t, string(bytes), "\"@version\":1")
		assert.Contains(t, string(bytes), "@timestamp")

		done <- struct{}{}
	}()

	entries := make(chan servicelog.Entry)
	logstash, err := NewLogstash(LogstashAddress("tcp", ln.Addr().String()))
	require.NoError(t, err)

	go logstash.Append(entries)

	entries <- servicelog.Entry{}
	<-done
}

func TestIfFormatsLogsCorrectly(t *testing.T) {
	logstash := logstash{}
	servicelogEntry := servicelog.Entry{}
	servicelogEntry["time"] = "time"
	servicelogEntry["msg"] = "log message"
	servicelogEntry["level"] = "WARNING"
	servicelogEntry["logger"] = "my logger"

	formattedEntry := logstash.formatEntry(servicelogEntry)

	assert.Equal(t, "time", formattedEntry["@timestamp"])
	assert.Equal(t, 1, formattedEntry["@version"])
	assert.Equal(t, "log message", formattedEntry["message"])
	assert.Equal(t, "WARNING", formattedEntry["level"])
	assert.Equal(t, "my logger", formattedEntry["logger"])
}

func TestIfFailsToStartWithInvalidLogstashConfiguration(t *testing.T) {
	_, err := NewLogstash(LogstashAddress("invalid", "!@#$"))
	assert.Error(t, err)
}

func TestIfFailsToStartWithInvalidLogstashConfigurationInEnv(t *testing.T) {
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_PROTOCOL", "invalid")
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_ADDRESS", "!@#$")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_PROTOCOL")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_ADDRESS")

	_, err := NewLogstash(LogstashAddressFromEnv())
	assert.Error(t, err)
}
