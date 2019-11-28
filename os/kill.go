// +build !windows

package os

import (
	"fmt"
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

	signals := wrapWithStopAndCont(signal, pgids)
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
func wrapWithStopAndCont(signal syscall.Signal, pgids []int) []syscall.Signal {
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
