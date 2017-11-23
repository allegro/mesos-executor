// +build darwin freebsd linux

package runenv

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOsHostnameReturnsSystemHostname(t *testing.T) {
	hostnameCommand = &mockCommand{stdout: "expected.hostname-prod.fqdn", err: nil}
	hostname, err := OsHostname()
	assert.NoError(t, err)
	assert.Equal(t, "expected.hostname-prod.fqdn", hostname)
}

func TestOsHostnameTrimsWhitespace(t *testing.T) {
	hostnameCommand = &mockCommand{stdout: "expected.hostname-prod.fqdn\n", err: nil}
	hostname, err := OsHostname()
	assert.NoError(t, err)
	assert.Equal(t, "expected.hostname-prod.fqdn", hostname)
}

func TestOsHostnameUsesGoDefaultHostnameWhenSystemHostnameEmpty(t *testing.T) {
	hostnameCommand = &mockCommand{stdout: "", err: nil}
	defaultHostname = func() (string, error) { return "defaultHost", nil }
	hostname, err := OsHostname()
	assert.NoError(t, err)
	assert.Equal(t, "defaultHost", hostname)
}

func TestOsHostnameUsesGoDefaultHostnameWhenSystemHostnameIsErroneous(t *testing.T) {
	hostnameCommand = &mockCommand{stdout: "", err: errors.New("Command failed")}
	defaultHostname = func() (string, error) { return "defaultHost", nil }
	hostname, err := OsHostname()
	assert.NoError(t, err)
	assert.Equal(t, "defaultHost", hostname)
}

type mockCommand struct {
	stdout string
	err    error
}

func (m *mockCommand) Run() (string, error) {
	return m.stdout, m.err
}
