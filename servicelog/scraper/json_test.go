package scraper

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIfScrapsLogsProperlyInJSONFormat(t *testing.T) {
	reader, writer := io.Pipe()
	scraper := JSON{}

	entries := scraper.StartScraping(reader)
	go writer.Write([]byte("{\"a\":\"b\", \"c\":\"d\"}\n"))

	entry := <-entries

	assert.Equal(t, "b", entry["a"])
	assert.Equal(t, "d", entry["c"])
}

func TestIfFiltersKeysFromScrapedJSONs(t *testing.T) {
	reader, writer := io.Pipe()
	scraper := JSON{
		KeyFilter: FilterFunc(func(v []byte) bool { return bytes.Equal(v, []byte("a")) }),
	}

	entries := scraper.StartScraping(reader)
	go writer.Write([]byte("{\"a\":\"b\", \"c\":\"d\"}\n"))

	entry := <-entries

	assert.Equal(t, "d", entry["c"])
	assert.Len(t, entry, 1)
}

func TestIfWrapsInDefaultValuesInvalidLogEntriesWhenEnabled(t *testing.T) {
	reader, writer := io.Pipe()
	scraper := JSON{
		ScrapUnmarshallableLogs: true,
	}

	entries := scraper.StartScraping(reader)
	go writer.Write([]byte("ERROR my invalid format\n"))

	entry := <-entries

	assert.Equal(t, "ERROR my invalid format", entry["msg"])
	assert.Equal(t, "invalid-format", entry["logger"])
	assert.Equal(t, "INFO", entry["level"])
}

func TestIfNotFailsWithTooLongTokens(t *testing.T) {
	reader, writer := io.Pipe()
	scraper := JSON{
		ScrapUnmarshallableLogs: true,
	}

	entries := scraper.StartScraping(reader)

	// send to long token (size > 1MB)
	go writer.Write(make([]byte, 1024*1024*5, 1024*1024*5))
	// send valid log, it should be scraped
	go writer.Write([]byte("{\"a\":\"b\", \"c\":\"d\"}\n"))

	entry := <-entries

	assert.Equal(t, "b", entry["a"])
	assert.Equal(t, "d", entry["c"])
	assert.Len(t, entry, 2)
}
