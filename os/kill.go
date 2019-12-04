// +build !windows

package os

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/shirou/gopsutil/process"
	log "github.com/sirupsen/logrus"
)

// KillTree sends signal to whole process tree, starting from given pid as root.
// Order of signalling in process tree is undefined.
func KillTree(signal syscall.Signal, pid int32) error {
	pgids, err := getProcessGroupsInTree(pid)
	if err != nil {
		return err
	}

	signals := wrapWithStopAndCont(signal)
	return sendSignalsToProcessGroups(signals, pgids)
}

func getProcessGroupsInTree(pid int32) ([]int, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	processes := getAllChildren(proc)
	processes = append(processes, proc)

	curPid := syscall.Getpid()
	curPgid, err := syscall.Getpgid(curPid)
	if err != nil {
		return nil, fmt.Errorf("error getting current process pgid: %s", err)
	}

	var pgids []int
	pgidsSeen := map[int]bool{}
	for _, proc := range processes {
		pgid, err := syscall.Getpgid(int(proc.Pid))
		if err != nil {
			return nil, fmt.Errorf("error getting child process pgid: %s", err)
		}
		if pgid == curPgid {
			continue
		}
		if !pgidsSeen[pgid] {
			pgids = append(pgids, pgid)
			pgidsSeen[pgid] = true
		}
	}
	return pgids, nil
}

// getAllChildren gets whole descendants tree of given process. Order of returned
// processes is undefined.
func getAllChildren(proc *process.Process) []*process.Process {
	children, _ := proc.Children() // #nosec

	for _, child := range children {
		children = append(children, getAllChildren(child)...)
	}

	return children
}

// wrapWithStopAndCont wraps original process tree signal sending with SIGSTOP and
// SIGCONT to prevent processes from forking during termination, so we will not
// have orphaned processes after.
func wrapWithStopAndCont(signal syscall.Signal) []syscall.Signal {
	signals := []syscall.Signal{syscall.SIGSTOP, signal}
	if signal != syscall.SIGKILL { // no point in sending any signal after SIGKILL
		signals = append(signals, syscall.SIGCONT)
	}
	return signals
}

func sendSignalsToProcessGroups(signals []syscall.Signal, pgids []int) error {
	for _, signal := range signals {
		for _, pgid := range pgids {
			log.Infof("Sending signal %s to pgid %d", signal, pgid)
			err := syscall.Kill(-pgid, signal)
			if err != nil {
				log.Infof("Error sending signal to pgid %d: %s", pgid, err)
			}
		}
	}
	return nil
}

// KillTreeWithExcludes sends signal to whole process tree, starting from given pid as root.
// Omits processes matching names specified in processesToExclude. Kills using pids instead of pgids.
func KillTreeWithExcludes(signal syscall.Signal, pid int32, processesToExclude []string) error {
	log.Infof("Will send signal %s to tree starting from %d", signal.String(), pid)

	if len(processesToExclude) == 0 {
		return KillTree(signal, pid)
	}

	pgids, err := getProcessGroupsInTree(pid)
	if err != nil {
		return err
	}

	log.Infof("Found process groups: %v", pgids)

	pids, err := findProcessesInGroups(pgids)
	if err != nil {
		return err
	}

	log.Infof("Found processes in groups: %v", pids)

	pids, err = excludeProcesses(pids, processesToExclude)
	if err != nil {
		return err
	}

	signals := wrapWithStopAndCont(signal)
	return sendSignalsToProcesses(signals, pids)
}

func findProcessesInGroups(pgids []int) ([]int, error) {
	var pids []int
	for _, pgid := range pgids {
		cmd := exec.Command("pgrep", "-g", strconv.Itoa(pgid))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("'pgrep -g %d' failed: %s", pgid, err)
		}
		if !cmd.ProcessState.Success() {
			return nil, fmt.Errorf("'pgrep -g %d' failed, output was: '%s'", pgid, output)
		}

		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			pid, err := strconv.Atoi(scanner.Text())
			if err != nil {
				return nil, fmt.Errorf("cannot convert pgrep output: %s. Output was '%s'", err, output)
			}
			pids = append(pids, pid)
		}
	}

	return pids, nil
}

func excludeProcesses(pids []int, processesToExclude []string) ([]int, error) {
	var retainedPids []int
	for _, pid := range pids {
		proc, err := process.NewProcess(int32(pid))
		if err != nil {
			return nil, err
		}

		name, err := proc.Name()
		if err != nil {
			log.Infof("Could not get process name of %d, continuing", pid)
			continue
		}

		if isExcluded(name, processesToExclude) {
			log.Infof("Excluding process %s with pid %d from kill", name, pid)
			continue
		}

		retainedPids = append(retainedPids, pid)
	}

	return retainedPids, nil
}

func isExcluded(name string, namesToExclude []string) bool {
	for _, exclude := range namesToExclude {
		if strings.ToLower(name) == strings.ToLower(exclude) {
			return true
		}
	}

	return false
}

func sendSignalsToProcesses(signals []syscall.Signal, pids []int) error {
	for _, signal := range signals {
		for _, pid := range pids {
			log.Infof("Sending signal %s to pid %d", signal, pid)
			err := syscall.Kill(pid, signal)
			if err != nil {
				log.Infof("Error sending signal to pid %d: %s", pid, err)
			}
		}
	}
	return nil
}
