// +build !windows

package os

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const TIME_LIMIT = 5 * time.Second // equal to sleep time in test scripts

func TestSendingSignalToTree(t *testing.T) {
	startTime := time.Now()
	cmd := exec.Command("testdata/fork.sh")
	err := cmd.Start()
	require.NoError(t, err)

	killErr := KillTree(syscall.SIGKILL, int32(cmd.Process.Pid))
	require.NoError(t, killErr)
	_, _ = cmd.Process.Wait()

	assert.False(t, processExists(cmd.Process.Pid))
	assertFinishedWithinTimeLimit(t, startTime)
}

// Test processes finish successfully after TIME_LIMIT. Only if the test finishes earlier
// can we be sure that the processes were indeed killed.
func assertFinishedWithinTimeLimit(t *testing.T, startTime time.Time) {
	assert.True(t, time.Now().Before(startTime.Add(TIME_LIMIT)),
		"took longer than %v, test processes were probably not killed", TIME_LIMIT)
}

func processExists(pid int) bool {
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}
