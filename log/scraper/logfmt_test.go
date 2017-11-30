package scraper

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIfScrapsLogsProperlyInLogFmtFormat(t *testing.T) {
	reader, writer := io.Pipe()
	scraper := LogFmt{}

	entries := scraper.StartScraping(reader)
	go writer.Write([]byte("a=b c=d\n"))

	entry := <-entries

	assert.Equal(t, "b", entry["a"])
	assert.Equal(t, "d", entry["c"])
}
