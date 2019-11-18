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
	proc, err := process.NewProcess(pid)
	if err != nil {
		return err
	}

	processes := getAllChildren(proc)
	processes = append(processes, proc)

	curPid := syscall.Getpid()
	curPgid, err := syscall.Getpgid(curPid)
	if err != nil {
		return fmt.Errorf("error getting current process pgid: %s", err)
	}

	var pgids []int
	pgidsSeen := map[int]bool{}
	for _, proc := range processes {
		pgid, err := syscall.Getpgid(int(proc.Pid))
		if err != nil {
			return fmt.Errorf("error getting child process pgid: %s", err)
		}
		if pgid == curPgid {
			continue
		}
		if !pgidsSeen[pgid] {
			pgids = append(pgids, pgid)
			pgidsSeen[pgid] = true
		}
	}

	return wrapWithStopAndCont(signal, pgids)
}

// KillTreeOmittingEnvoy sends signal to whole process tree, killing by pids instead of pgids.
// Omits Envoy processes.
func KillTreeOmittingEnvoy(signal syscall.Signal, pid int32) error {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return err
	}

	processes := getAllChildren(proc)
	processes = append(processes, proc)

	processes = removeProcessesMatching(processes, func(p *process.Process) bool {
		name, _ := p.Name()
		return name == "envoy"
	})

	var pids []int
	for _, proc := range processes {
		pids = append(pids, int(proc.Pid))
	}

	signals := []syscall.Signal{syscall.SIGSTOP, signal, syscall.SIGCONT}

	return sendSignalsToProcesses(signals, pids)
}

func removeProcessesMatching(all []*process.Process, shouldOmit func(*process.Process) bool) []*process.Process {
	var retained []*process.Process
	for _, p := range all {
		if shouldOmit(p) {
			log.Printf("omitting process %d in KillTree", p.Pid)
			continue
		}
		retained = append(retained, p)
	}

	return retained
}

func sendSignalsToProcesses(signals []syscall.Signal, pids []int) error {
	for _, signal := range signals {
		for _, pid := range pids {
			log.Infof("Sending signal %s to pid %d", signal, pid)
			err := syscall.Kill(pid, signal)
			if err != nil {
				log.Infof("Error sending signal to pid %d: %s", pid, err)
				return err
			}
		}
	}
	return nil
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
func wrapWithStopAndCont(signal syscall.Signal, pgids []int) error {
	signals := []syscall.Signal{syscall.SIGSTOP, signal, syscall.SIGCONT}
	for _, currentSignal := range signals {
		if err := sendSignalToProcessGroups(currentSignal, pgids); err != nil {
			return err
		}
	}
	return nil
}

func sendSignalToProcessGroups(signal syscall.Signal, pgids []int) error {
	for _, pgid := range pgids {
		log.Infof("Sending signal %s to pgid %d", signal, pgid)
		err := syscall.Kill(-pgid, signal)
		if err != nil {
			log.Infof("Error sending signal to pgid %d: %s", pgid, err)
			return err
		}
	}
	return nil
}
