// +build !windows

package os

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendingSignalToTree(t *testing.T) {
	cmd := exec.Command("testdata/fork.sh")
	err := cmd.Start()
	require.NoError(t, err)

	killErr := KillTree(syscall.SIGKILL, int32(cmd.Process.Pid))
	require.NoError(t, killErr)
	_, _ = cmd.Process.Wait()

	assert.False(t, processExists(cmd.Process.Pid))
}

func processExists(pid int) bool {
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}
