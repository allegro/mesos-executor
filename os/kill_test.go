// +build !windows

package os

import (
	"errors"
	"github.com/shirou/gopsutil/process"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const TIME_LIMIT = 3 * time.Second // equal to sleep time in test scripts

func TestKillTree_SimpleTreeTree(t *testing.T) {
	startTime := time.Now()
	cmd := startTestProcesses(t, "testdata/fork.sh")
	cmdPids := addAllChildrenPids(cmd.Process.Pid)

	killErr := KillTree(syscall.SIGKILL, int32(cmd.Process.Pid))
	require.NoError(t, killErr)
	waitToDie(cmd)

	assertProcessesDontExist(t, cmdPids)
	assertFinishedWithinTimeLimit(t, startTime)
}

func TestKillTree_ComplexTree(t *testing.T) {
	startTime := time.Now()
	cmd := startTestProcesses(t, "testdata/fork2.sh")
	cmdPids := addAllChildrenPids(cmd.Process.Pid)

	killErr := KillTree(syscall.SIGKILL, int32(cmd.Process.Pid))
	require.NoError(t, killErr)
	waitToDie(cmd)

	assertProcessesDontExist(t, cmdPids)
	assertFinishedWithinTimeLimit(t, startTime)
}

func TestKillTreeWithExcludes_SimpleTree(t *testing.T) {
	startTime := time.Now()
	cmd := startTestProcesses(t, "testdata/fork.sh")
	cmdPids := addAllChildrenPids(cmd.Process.Pid)

	killErr := KillTreeWithExcludes(syscall.SIGTERM, int32(cmd.Process.Pid), []string{})
	require.NoError(t, killErr)
	waitToDie(cmd)

	assertProcessesDontExist(t, cmdPids)
	assertFinishedWithinTimeLimit(t, startTime)
}

func TestKillTreeWithExcludes_ComplexTree(t *testing.T) {
	startTime := time.Now()
	cmd := startTestProcesses(t, "testdata/fork2.sh")
	cmdPids := addAllChildrenPids(cmd.Process.Pid)

	killErr := KillTreeWithExcludes(syscall.SIGTERM, int32(cmd.Process.Pid), []string{})
	require.NoError(t, killErr)
	waitToDie(cmd)

	assertProcessesDontExist(t, cmdPids)
	assertFinishedWithinTimeLimit(t, startTime)
}

func TestKillTreeWithExcludes_ComplexTreeExcludingProcessThatDoesntExist(t *testing.T) {
	startTime := time.Now()
	cmd := startTestProcesses(t, "testdata/fork2.sh")
	cmdPids := addAllChildrenPids(cmd.Process.Pid)

	killErr := KillTreeWithExcludes(syscall.SIGTERM, int32(cmd.Process.Pid), []string{"non-existing"})
	require.NoError(t, killErr)
	waitToDie(cmd)

	assertProcessesDontExist(t, cmdPids)
	assertFinishedWithinTimeLimit(t, startTime)
}

func TestKillTreeWithExcludes_ComplexTreeExcludingOneExistingProcess(t *testing.T) {
	startTime := time.Now()
	cmd := startTestProcesses(t, "testdata/fork2.sh")
	cmdPids := addAllChildrenPids(cmd.Process.Pid)
	excluded, findErr := findPidWithProcessName("python", cmdPids)
	require.NoError(t, findErr)

	killErr := KillTreeWithExcludes(syscall.SIGTERM, int32(cmd.Process.Pid), []string{"python"})
	require.NoError(t, killErr)
	waitToDie(cmd)

	assertProcessesDontExist(t, removePid(cmdPids, excluded))
	assertProcessExists(t, excluded)
	assertFinishedWithinTimeLimit(t, startTime)
}

func TestKillTreeWithExcludes_ComplexTreeExcludingOneExistingProcessAndOneNonExisting(t *testing.T) {
	startTime := time.Now()
	cmd := startTestProcesses(t, "testdata/fork2.sh")
	cmdPids := addAllChildrenPids(cmd.Process.Pid)
	excluded, findErr := findPidWithProcessName("python", cmdPids)
	require.NoError(t, findErr)

	killErr := KillTreeWithExcludes(syscall.SIGTERM, int32(cmd.Process.Pid), []string{"python","non-existing"})
	require.NoError(t, killErr)
	waitToDie(cmd)

	assertProcessesDontExist(t, removePid(cmdPids, excluded))
	assertProcessExists(t, excluded)
	assertFinishedWithinTimeLimit(t, startTime)
}

func startTestProcesses(t *testing.T, commandName string) *exec.Cmd {
	cmd := exec.Command(commandName)
	err := cmd.Start()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond) // give time for processes to spawn
	return cmd
}

func addAllChildrenPids(rootPid int) []int {
	newProcess, _ := process.NewProcess(int32(rootPid))
	children := getAllChildren(newProcess)
	pids := []int{rootPid}
	for _, c := range children {
		pids = append(pids, int(c.Pid))
	}
	return pids
}

func findPidWithProcessName(searchedName string, pids []int) (int, error) {
	for _, pid := range pids {
		proc, err := process.NewProcess(int32(pid))
		if err != nil {
			return -1, err
		}
		name, err := proc.Name()
		if err != nil {
			return -1, err
		}
		if strings.ToLower(name) == searchedName {
			return pid, nil
		}
	}
	return -1, errors.New("process not found")
}

func waitToDie(cmd *exec.Cmd) {
	time.Sleep(100 * time.Millisecond) // give time for children to die
	_, _ = cmd.Process.Wait()
}

func removePid(pids []int, toRemove int) []int {
	var result []int
	for _, pid := range pids {
		if pid != toRemove {
			result = append(result, pid)
		}
	}
	return result
}

func assertProcessesDontExist(t *testing.T, cmdPids []int) {
	for _, pid := range cmdPids {
		assert.False(t, processExists(pid), "process %d still exists", pid)
	}
}

func assertProcessExists(t *testing.T, pid int) {
	assert.True(t, processExists(pid), "process %d does not exist", pid)
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
