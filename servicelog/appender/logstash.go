package appender

import (
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/allegro/mesos-executor/xio"
	"github.com/kelseyhightower/envconfig"
	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor/servicelog"
)

const (
	logstashVersion      = 1
	logstashConfigPrefix = "allegro_executor_servicelog_logstash"
)

type logstashConfig struct {
	Protocol string `required:"true"`
	Address  string `required:"true"`
}

type logstashEntry map[string]interface{}

type logstash struct {
	writer io.Writer
}

func (l logstash) Append(entries <-chan servicelog.Entry) {
	for entry := range entries {
		if err := l.sendEntry(entry); err != nil {
			log.WithError(err).Warn("Error appending logs.")
		}
	}
}

func (l logstash) formatEntry(entry servicelog.Entry) logstashEntry {
	formattedEntry := logstashEntry{}
	formattedEntry["@timestamp"] = entry["time"]
	formattedEntry["@version"] = logstashVersion
	formattedEntry["message"] = entry["msg"]

	for key, value := range entry {
		if key == "msg" || key == "time" {
			continue
		}
		formattedEntry[key] = value
	}

	return formattedEntry
}

func (l logstash) sendEntry(entry servicelog.Entry) error {
	formattedEntry := l.formatEntry(entry)
	bytes, err := l.marshal(formattedEntry)
	if err != nil {
		return fmt.Errorf("unable to marshal log entry: %s", err)
	}
	log.WithField("entry", string(bytes)).Debug("Sending log entry to Logstash")
	if _, err = l.writer.Write(bytes); err != nil {
		return fmt.Errorf("unable to write to Logstash server: %s", err)
	}
	return nil
}

func (l logstash) marshal(entry logstashEntry) ([]byte, error) {
	bytes, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	// Logstash reads logs line by line so we need to add a newline after each
	// generated JSON entry
	bytes = append(bytes, '\n')
	return bytes, nil
}

// NewLogstash creates new appender that will send log entries to Logstash using
// passed writer.
func NewLogstash(writer io.Writer, options ...func(*logstash) error) (Appender, error) {
	l := &logstash{
		writer: writer,
	}
	for _, option := range options {
		if err := option(l); err != nil {
			return nil, fmt.Errorf("invalid config option: %s", err)
		}
	}
	return l, nil
}

// LogstashWriterFromEnv creates the connection from the environment  variables
// for the Logstash appender.
func LogstashWriterFromEnv() (io.Writer, error) {
	config := &logstashConfig{}
	err := envconfig.Process(logstashConfigPrefix, config)
	if err != nil {
		return nil, fmt.Errorf("unable to get address from env: %s", err)
	}
	return net.Dial(config.Protocol, config.Address)
}

// LogstashRateLimit adds rate limiting to logs sending. Logs send in higher rate
// (log lines per seconds) will be discarded.
func LogstashRateLimit(limit int) func(*logstash) error {
	return func(l *logstash) error {
		l.writer = xio.DecorateWriter(l.writer, xio.RateLimit(limit))
		return nil
	}
}

// LogstashSizeLimit adds size limiting to logs sending. Logs that exceeds passed
// size (in bytes) will be discarded.
func LogstashSizeLimit(size int) func(*logstash) error {
	return func(l *logstash) error {
		l.writer = xio.DecorateWriter(l.writer, xio.SizeLimit(size))
		return nil
	}
}
