package scraper

import (
	"bufio"
	"encoding/json"
	"io"

	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor/servicelog"
)

// JSON is a scraper for logs represented as JSON objects.
type JSON struct {
}

// StartScraping starts scraping logs in JSON format from given reader and sends
// parsed entries to the returned unbuffered channel. Logs are scraped as long
// as the passed reader does not return an io.EOF error.
func (j *JSON) StartScraping(reader io.Reader) <-chan servicelog.Entry {
	scanner := bufio.NewScanner(reader)
	logEntries := make(chan servicelog.Entry)

	go func() {
		for scanner.Scan() {
			logEntry := servicelog.Entry{}
			if err := json.Unmarshal(scanner.Bytes(), &logEntry); err != nil {
				log.WithError(err).Warn("Unable to unmarshal log entry - skipping line")
				continue
			}
			logEntries <- logEntry
		}
		log.WithError(scanner.Err()).Error("Service log scraping failed")
	}()

	return logEntries
}
