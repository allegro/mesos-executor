package runenv

import "os"

// OsHostname returns os.Hostname() on windows
func OsHostname() (string, error) {
	return os.Hostname()
}
