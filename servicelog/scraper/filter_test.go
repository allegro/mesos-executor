package scraper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIfFiltersConfiguredValues(t *testing.T) {
	testCases := []struct {
		filteredValues [][]byte
		value          []byte
		match          bool
	}{
		{[][]byte{[]byte("a")}, []byte("a"), true},
		{[][]byte{[]byte("a")}, []byte("b"), false},
		{[][]byte{[]byte("a"), []byte("b"), []byte("c")}, []byte("d"), false},
		{[][]byte{[]byte("a"), []byte("b"), []byte("c")}, []byte("a"), true},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("filter=%s;value=%s", tc.filteredValues, tc.value), func(t *testing.T) {
			filter := ValueFilter{
				Values: tc.filteredValues,
			}
			match := filter.Match(tc.value)
			assert.Equal(t, tc.match, match)
		})
	}
}
