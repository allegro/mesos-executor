package scraper

import (
	"io"

	"github.com/allegro/mesos-executor/servicelog"
)

// Scraper in an interface for various scrapers that support different log formats.
type Scraper interface {
	StartScraping(io.Reader) <-chan servicelog.Entry
}

// Pipe returns a channel with log entries and writer that can be used as a data
// provider for given scraper.
func Pipe(scraper Scraper) (<-chan servicelog.Entry, io.Writer) {
	reader, writer := io.Pipe()
	entries := scraper.StartScraping(reader)
	return entries, writer
}
