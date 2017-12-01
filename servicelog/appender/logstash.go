package appender

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/kelseyhightower/envconfig"

	"github.com/allegro/mesos-executor/servicelog"
)

const (
	logstashVersion           = "1"
	logstashConfigPrefix      = "allegro_executor_servicelog_logstash"
	logstashDefaultTimeFormat = time.RFC3339Nano
)

type logstashConfig struct {
	Protocol string `required:"true"`
	Address  string `required:"true"`
}

type logstash struct {
	conn net.Conn
}

func (l logstash) Append(entries <-chan servicelog.Entry) {
	for entry := range entries {
		if err := l.sendEntry(entry); err != nil {
			log.WithError(err).Warn("Error appending logs.")
		}
	}
}

func (l logstash) sendEntry(entry servicelog.Entry) error {
	// TODO(medzin): Move formatting logic to separate structure and extend it there
	entry["@timestamp"] = time.Now().Format(logstashDefaultTimeFormat)
	entry["@version"] = logstashVersion
	bytes, err := json.Marshal(entry)
	bytes = append(bytes, '\n')

	if err != nil {
		return fmt.Errorf("unable to marshal log entry: %s", err)
	}
	if _, err = l.conn.Write(bytes); err != nil {
		return fmt.Errorf("unable to write to Logstash server: %s", err)
	}
	return nil
}

// NewLogstash creates new appender that will send log entries to Logstash.
func NewLogstash(options ...func(*logstash) error) (Appender, error) {
	l := &logstash{}
	for _, option := range options {
		if err := option(l); err != nil {
			return nil, fmt.Errorf("invalid config option: %s", err)
		}
	}
	return l, nil
}

// LogstashAddress sets the connection details for the Logstash appender.
func LogstashAddress(protocol, address string) func(*logstash) error {
	return func(l *logstash) error {
		conn, err := net.Dial(protocol, address)
		if err != nil {
			return fmt.Errorf("unable to connect to Logstash server: %s", err)
		}
		l.conn = conn
		return nil
	}
}

// LogstashAddressFromEnv sets the connection details from the environment
// variables for the Logstash appender.
func LogstashAddressFromEnv() func(*logstash) error {
	config := &logstashConfig{}
	err := envconfig.Process(logstashConfigPrefix, config)
	if err != nil {
		return func(l *logstash) error {
			return fmt.Errorf("unable to get address from env: %s", err)
		}
	}
	return LogstashAddress(config.Protocol, config.Address)
}
