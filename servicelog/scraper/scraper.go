package scraper

import (
	"io"

	"github.com/allegro/mesos-executor/servicelog"
)

// Scraper in an interface for various scrapers that support different log formats.
type Scraper interface {
	StartScraping(io.Reader) <-chan servicelog.Entry
}

// Pipe returns writer that can be used as a data provider for given scraper.
func Pipe(scraper Scraper) io.Writer {
	reader, writer := io.Pipe()
	scraper.StartScraping(reader)
	return writer
}
