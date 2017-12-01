package appender

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/allegro/mesos-executor/servicelog"
)

const (
	logstashVersion           = "1"
	logstashDefaultTimeFormat = time.RFC3339Nano
)

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
func NewLogstash(protocol, address string) (Appender, error) {
	conn, err := net.Dial(protocol, address)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to Logstash server: %s", err)
	}
	return &logstash{conn: conn}, nil
}
