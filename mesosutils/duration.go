package mesosutils

import "time"

// Duration translates float second values used by Marathon to go Duration format
func Duration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
