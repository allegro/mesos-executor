package appender

import (
	"encoding/json"
	"fmt"
	"net"

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
	conn net.Conn
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
	if _, err = l.conn.Write(bytes); err != nil {
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
