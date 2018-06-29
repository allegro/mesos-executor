package appender

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/allegro/mesos-executor/servicelog"
	"github.com/allegro/mesos-executor/xnet"
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
	writer, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	logstash, err := NewLogstash(writer)
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

func TestIfCreatesAppenderWithValidDiscoveryConfigurationInEnv(t *testing.T) {
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL", "tcp")
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_DISCOVERY_SERVICE_NAME", "logstash")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_DISCOVERY_SERVICE_NAME")

	logstash, err := LogstashAppenderFromEnv()

	assert.NoError(t, err)
	assert.NotNil(t, logstash)
}

func TestIfCreatesAppenderWithValidStaticAddressConfigurationInEnv(t *testing.T) {
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL", "udp")
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_ADDRESS", "localhost:12345")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_ADDRESS")

	logstash, err := LogstashAppenderFromEnv()

	assert.NoError(t, err)
	assert.NotNil(t, logstash)
}

func TestIfFailsToCreateAppenderWithInvalidRequiredConfigurationInEnv(t *testing.T) {
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL", "invalid")
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_ADDRESS", "!@#$")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_ADDRESS")

	_, err := LogstashAppenderFromEnv()
	assert.Error(t, err)
}

func TestIfFailsToCreateAppenderWithInvalidOptionalConfigurationInEnv(t *testing.T) {
	// valid required env
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL", "udp")
	os.Setenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_ADDRESS", "localhost:8080")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL")
	defer os.Unsetenv("ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_ADDRESS")

	testCases := []struct {
		envKey, envVal string
	}{
		{"ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_RATE_LIMIT", "invalid"},
		{"ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_SIZE_LIMIT", "invalid"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("envKey=%s;envVal=%s", tc.envKey, tc.envVal), func(t *testing.T) {
			// invalid optional env
			os.Setenv(tc.envKey, tc.envVal)
			defer os.Unsetenv(tc.envKey)
			_, err := LogstashAppenderFromEnv()
			assert.Error(t, err)
		})
	}
}

// func BenchmarkLogstash_Append(b *testing.B) {
// 	ln, err := net.Listen("tcp", "127.0.0.1:0")
// 	if err != nil {
// 		b.Fatal(err)
// 	}

// 	go func() {
// 		defer ln.Close()
// 		conn, err := ln.Accept()
// 		if err != nil {
// 			b.Fatal(err)
// 		}
// 		defer conn.Close()

// 		buff := make([]byte, 1024, 1024)
// 		for {
// 			_, err := conn.Read(buff)
// 			if err != nil {
// 				b.Fatal(err)
// 			}
// 		}
// 	}()

// 	entries := make(chan servicelog.Entry)
// 	writer, _ := net.Dial("tcp", ln.Addr().String())
// 	logstash, err := NewLogstash(writer)
// 	if err != nil {
// 		b.Fatal(err)
// 	}
// 	go logstash.Append(entries)
// 	for n := 0; n < b.N; n++ {
// 		entries <- servicelog.Entry{"msg": "message"}
// 	}
// }

func BenchmarkLogstash_Append(b *testing.B) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}

	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			b.Fatal(err)
		}
		defer conn.Close()

		buff := make([]byte, 1024, 1024)
		for {
			_, err := conn.Read(buff)
			if err != nil {
				b.Fatal(err)
			}
		}
	}()

	entries := make(chan servicelog.Entry)
	instanceProvider := make(chan []xnet.Address, 1)
	instanceProvider <- []xnet.Address{xnet.Address(ln.Addr().String())}
	sender := &xnet.TCPSender{}
	writer := xnet.BufferedRoundRobinWriter(instanceProvider, sender, 1000)
	logstash, err := NewLogstash(writer)
	if err != nil {
		b.Fatal(err)
	}
	go logstash.Append(entries)
	for n := 0; n < b.N; n++ {
		entries <- servicelog.Entry{"msg": "message"}
	}
}
