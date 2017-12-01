package scraper

import (
	"io"

	"github.com/go-logfmt/logfmt"

	"github.com/allegro/mesos-executor/servicelog"
)

// LogFmt is a scraper for logs in logfmt format.
//
// See: https://brandur.org/logfmt
type LogFmt struct {
}

// StartScraping starts scraping logs in logfmt format from given reader and sends
// parsed entries to the returned unbuffered channel. Logs are scraped as long
// as the passed reader does not return an io.EOF error.
func (logFmt *LogFmt) StartScraping(reader io.Reader) <-chan servicelog.Entry {
	decoder := logfmt.NewDecoder(reader)
	logEntries := make(chan servicelog.Entry)

	go func() {
		for decoder.ScanRecord() {
			logEntry := servicelog.Entry{}

			for decoder.ScanKeyval() {
				key := string(decoder.Key())
				value := string(decoder.Value())
				logEntry[key] = value
			}

			logEntries <- logEntry
		}
	}()

	return logEntries
}
