package scraper

import "bytes"

// Filter is an interface that performs filtering tasks during log scraping. It
// allows to ignore log values based on implementation logic.
type Filter interface {
	Match([]byte) bool
}

// The FilterFunc type is an adapter to allow the use of ordinary functions as
// scraping filters. If f is a function with the appropriate signature,
// FilterFunc(f) is a Filter that calls f.
type FilterFunc func([]byte) bool

// Match calls f(value).
func (f FilterFunc) Match(value []byte) bool {
	return f(value)
}

// ValueFilter allows to ignore specific values during log scraping.
type ValueFilter struct {
	Values [][]byte
}

// Match returns true if passed value is on the filtered list - false otherwise.
func (f ValueFilter) Match(v []byte) bool {
	for _, value := range f.Values {
		if bytes.Equal(v, value) {
			return true
		}
	}
	return false
}
