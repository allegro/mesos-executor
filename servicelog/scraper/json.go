package scraper

import (
	"bufio"
	"io"
	"os"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/rcrowley/go-metrics"
	log "github.com/sirupsen/logrus"

	"fmt"

	"github.com/allegro/mesos-executor/servicelog"
)

const (
	kilobyte = 1024
	megabyte = 1024 * kilobyte
)

var json = jsoniter.ConfigFastest

// JSON is a scraper for logs represented as JSON objects.
type JSON struct {
	InvalidLogsWriter              io.Writer
	KeyFilter                      Filter
	BufferSize                     uint
	ScrapUnmarshallableLogs        bool
	droppedBecauseOfBufferOverflow metrics.Counter
	receivedLogsTotal              metrics.Counter
}

// StartScraping starts scraping logs in JSON format from given reader and sends
// parsed entries to the returned unbuffered channel. Logs are scraped as long
// as the passed reader does not return an io.EOF error.
func (j *JSON) StartScraping(reader io.Reader) <-chan servicelog.Entry {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*kilobyte), megabyte)
	logEntries := make(chan servicelog.Entry, j.BufferSize)

	j.receivedLogsTotal = metrics.GetOrRegisterCounter(
		"servicelog.received.Total", metrics.DefaultRegistry)
	j.droppedBecauseOfBufferOverflow = metrics.GetOrRegisterCounter(
		"servicelog.scrapped.dropped.BufferOverflow", metrics.DefaultRegistry)

	go func() {
		for {
			err := j.scanLoop(reader, logEntries)
			log.WithError(err).Warn("Service log scraping failed, restarting")
		}
	}()

	return logEntries
}

func (j *JSON) scanLoop(reader io.Reader, logEntries chan<- servicelog.Entry) error {
	var invalidLogsWriter io.Writer = os.Stdout
	if j.InvalidLogsWriter != nil {
		invalidLogsWriter = j.InvalidLogsWriter
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*kilobyte), megabyte)
	for scanner.Scan() {
		j.receivedLogsTotal.Inc(1)
		logEntry := servicelog.Entry{}
		if err := json.Unmarshal(scanner.Bytes(), &logEntry); err != nil {
			if j.ScrapUnmarshallableLogs {
				log.WithError(err).Debug("Unable to unmarshal log entry - wrapping in default entry")
				logEntry = j.wrapInDefault(scanner.Bytes())
			} else {
				if _, err = fmt.Fprintf(invalidLogsWriter, "%s\n", scanner.Bytes()); err != nil {
					log.WithError(err).Error("unable to print out unmarshallable log")
				}

				continue
			}
		} else if j.KeyFilter != nil {
			for key := range logEntry {
				if j.KeyFilter.Match([]byte(key)) {
					delete(logEntry, key)
				}
			}
		}
		if j.BufferSize > 0 && len(logEntries) >= int(j.BufferSize) {
			j.droppedBecauseOfBufferOverflow.Inc(1)
			continue
		}
		logEntries <- logEntry
	}
	return scanner.Err()
}

func (j *JSON) wrapInDefault(bytes []byte) servicelog.Entry {
	return servicelog.Entry{
		"time":   time.Now().Format(time.RFC3339Nano),
		"level":  "INFO",
		"logger": "invalid-format",
		"msg":    string(bytes),
	}
}
