// +build darwin freebsd linux

package runenv

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var hostnameCommand execCommand = execHostnameFqdn{}
var defaultHostname = os.Hostname

// OsHostname returns result of calling hostname -f command, resorting to golang's os package if necessary
func OsHostname() (string, error) {
	fqdn, err := hostnameCommand.Run()
	if err != nil || len(fqdn) == 0 {
		return defaultHostname()
	}

	return strings.TrimSpace(fqdn), nil
}

type execCommand interface {
	Run() (string, error)
}

type execHostnameFqdn struct{}

func (execHostnameFqdn) Run() (string, error) {
	cmd := exec.Command("hostname", "-f") // nolint: gas
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	if err != nil {
		return "", fmt.Errorf("couldn't run 'hostname -f': %s", err)
	}

	return out.String(), nil
}
