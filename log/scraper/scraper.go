package scraper

import "io"

// LogEntry represents one scraped log line in flat key-value store.
type LogEntry map[string]string

// Scraper in an interface for various scrapers that support different log formats.
type Scraper interface {
	StartScraping(io.Reader) <-chan LogEntry
}
