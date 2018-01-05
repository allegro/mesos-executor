package xio

import (
	"errors"
	"io"

	"golang.org/x/time/rate"
)

var (
	// ErrRateLimitExceeded is returned by decorated io.Writer for which the rate
	// limit has been exceeded.
	ErrRateLimitExceeded = errors.New("rate of writing bytes exceeded")
	// ErrSizeLimitExceeded is returned by decorated io.Writer for which the size
	// limit has been exceeded.
	ErrSizeLimitExceeded = errors.New("limit of written bytes exceeded")
)

// WriterDecorator is a type representing functions that can be used to add
// (to decorate writer with) functionality to the passed writer.
type WriterDecorator func(io.Writer) io.Writer

// WriterFunc type is an adapter to allow the use of ordinary functions as
// io.Writers. If f is a function with the appropriate signature, WriterFunc(f)
// is a Writer that calls f.
type WriterFunc func([]byte) (int, error)

func (w WriterFunc) Write(p []byte) (int, error) {
	return w(p)
}

// DecorateWriter returns writer, based on the passed one, decorated with passed
// decorators.
func DecorateWriter(writer io.Writer, decorators ...WriterDecorator) io.Writer {
	for _, decorator := range decorators {
		writer = decorator(writer)
	}
	return writer
}

// RateLimit decorator is used to add a rate limit (number of Write calls per
// second) for io.Writer. If write rate exceeds the passed limit it will return
// an error.
func RateLimit(limit int) WriterDecorator {
	limiter := rate.NewLimiter(rate.Limit(limit), limit)
	return func(writer io.Writer) io.Writer {
		return WriterFunc(func(p []byte) (int, error) {
			if !limiter.Allow() {
				return 0, ErrRateLimitExceeded
			}
			return writer.Write(p)
		})
	}
}

// SizeLimit decorator is used to add a size limit (number of bytes) for io.Writer.
// If written bytes length exceeds the passed limit it will return an error.
func SizeLimit(size int) WriterDecorator {
	return func(writer io.Writer) io.Writer {
		return WriterFunc(func(p []byte) (int, error) {
			if len(p) > size {
				return 0, ErrSizeLimitExceeded
			}
			return writer.Write(p)
		})
	}
}
