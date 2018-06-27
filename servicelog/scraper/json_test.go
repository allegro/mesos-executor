package scraper

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/allegro/mesos-executor/servicelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func TestIfPrintsToStdoutValuesInvalidLogEntriesWhenDisabled(t *testing.T) {
	mockStdout := &mockWriter{}
	mockStdout.On("Write", []byte("ERROR my invalid format\n")).Return(0, nil).Once()

	reader, writer := io.Pipe()
	scraper := JSON{
		InvalidLogsWriter: mockStdout,
	}

	entries := scraper.StartScraping(reader)
	go writer.Write([]byte("ERROR my invalid format\n"))
	err := noEntryWithTimeout(entries, time.Millisecond)

	assert.NoError(t, err)
	mockStdout.AssertExpectations(t)
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

func TestIfIgnoresEmptyLogLines(t *testing.T) {
	reader, writer := io.Pipe()
	scraper := JSON{}

	entries := scraper.StartScraping(reader)

	go writer.Write([]byte("\n"))
	err1 := noEntryWithTimeout(entries, time.Millisecond)
	assert.NoError(t, err1)

	go writer.Write([]byte("  \t\n"))
	err2 := noEntryWithTimeout(entries, time.Millisecond)
	assert.NoError(t, err2)
}

func noEntryWithTimeout(entries <-chan servicelog.Entry, timeout time.Duration) error {
	timeoutChan := time.After(timeout)
	select {
	case entry, ok := <-entries:
		if ok {
			return fmt.Errorf("entry %s was read before timeout", entry)
		}
		return errors.New("channel closed before timeout")
	case <-timeoutChan:
		return nil
	}
}

type mockWriter struct {
	mock.Mock
}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	args := w.Called(p)
	return args.Int(0), args.Error(1)
}
