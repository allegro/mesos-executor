// +build !windows

package executor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	osutil "github.com/allegro/mesos-executor/os"
	"github.com/allegro/mesos-executor/servicelog"
	"github.com/allegro/mesos-executor/servicelog/appender"
	"github.com/allegro/mesos-executor/servicelog/scraper"
	"github.com/mesos/mesos-go/api/v1/lib"
)

// TaskExitState is a type describing reason of program execution interuption.
type TaskExitState struct {
	Code TaskExitCode
	Err  error
}

// TaskExitCode is an enum.
type TaskExitCode int8

const (
	// SuccessCode means task exited successfully.
	SuccessCode TaskExitCode = iota
	// FailedCode means task exited with error.
	FailedCode
	// KilledCode means task was killed and it's code was ignored.
	KilledCode
)

// Command is an interface to abstract command running on a system.
type Command interface {
	Start() error
	Wait() <-chan TaskExitState
	Stop(gracePeriod time.Duration)
}

type cancellableCommand struct {
	cmd      *exec.Cmd
	doneChan chan error
	killing  bool
}

func (c *cancellableCommand) Start() error {
	if c.cmd == nil {
		return errors.New("missing command to run")
	}

	if err := c.cmd.Start(); err != nil {
		return err
	}

	c.doneChan = make(chan error)
	go c.waitForCommand()

	return nil
}

func (c *cancellableCommand) Wait() <-chan TaskExitState {
	exitChan := make(chan TaskExitState)

	go func() {
		err := <-c.doneChan

		log.Infof("Command exited with state: %s", c.cmd.ProcessState.String())

		if err == nil && c.cmd.ProcessState.Success() {
			exitChan <- TaskExitState{
				Code: SuccessCode,
			}
			return
		}
		if c.killing {
			exitChan <- TaskExitState{
				Code: KilledCode,
			}
			return
		}

		exitChan <- TaskExitState{
			Code: FailedCode,
			Err:  err,
		}
	}()

	return exitChan
}

func (c *cancellableCommand) waitForCommand() {
	err := c.cmd.Wait()
	c.doneChan <- err
	close(c.doneChan)
}

func (c *cancellableCommand) Stop(gracePeriod time.Duration) {
	// Return if Stop was already called.
	if c.killing {
		return
	}
	c.killing = true
	err := osutil.KillTree(syscall.SIGTERM, int32(c.cmd.Process.Pid))
	if err != nil {
		log.WithError(err).Errorf("There was a problem with sending %s to %d children", syscall.SIGTERM, c.cmd.Process.Pid)
		return
	}

	<-time.After(gracePeriod)

	if err := osutil.KillTree(syscall.SIGKILL, int32(c.cmd.Process.Pid)); err != nil {
		log.WithError(err).Warnf("There was a problem with sending %s to %d tree", syscall.SIGKILL, c.cmd.Process.Pid)
		return
	}
}

// NewCommand returns a new command based on passed CommandInfo.
func NewCommand(commandInfo mesos.CommandInfo, env []string, options ...func(*exec.Cmd) error) (Command, error) {
	// TODO(janisz): Implement shell policy
	// From: https://github.com/apache/mesos/blob/1.1.3/include/mesos/mesos.proto#L509-L521
	// There are two ways to specify the command:
	// 1) If 'shell == true', the command will be launched via shell
	//		(i.e., /bin/sh -c 'value'). The 'value' specified will be
	//		treated as the shell command. The 'arguments' will be ignored.
	// 2) If 'shell == false', the command will be launched by passing
	//		arguments to an executable. The 'value' specified will be
	//		treated as the filename of the executable. The 'arguments'
	//		will be treated as the arguments to the executable. This is
	//		similar to how POSIX exec families launch processes (i.e.,
	//		execlp(value, arguments(0), arguments(1), ...)).
	cmd := exec.Command("sh", "-c", commandInfo.GetValue()) // #nosec
	cmd.Env = append(envWithoutExecutorConfig(), env...)
	for _, option := range options {
		if err := option(cmd); err != nil {
			return nil, fmt.Errorf("invalid config option: %s", err)
		}
	}
	// Set new group for a command
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return &cancellableCommand{cmd: cmd}, nil
}

// ForwardCmdOutput configures command to forward its output to the system stderr
// and stdout.
func ForwardCmdOutput() func(*exec.Cmd) error {
	return func(cmd *exec.Cmd) error {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		return nil
	}
}

// ScrapCmdOutput configures command so itd output will be scraped and forwarded
// by provided log appender.
func ScrapCmdOutput(s scraper.Scraper, a appender.Appender, extenders ...servicelog.Extender) func(*exec.Cmd) error {
	return func(cmd *exec.Cmd) error {
		entries, writer := scraper.Pipe(s)
		entries = servicelog.Extend(entries, extenders...)
		cmd.Stderr = writer
		cmd.Stdout = writer
		go a.Append(entries)
		return nil
	}
}

// envWithoutExecutorConfig returns os.Environ without executor specific entries.
// Marathon does not support custom executor env and all task env are passed
// as executor env. This means environment are setup before executor startup.
func envWithoutExecutorConfig() (env []string) {
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, strings.ToUpper(EnvironmentPrefix)) {
			env = append(env, variable)
		} else {

		}
	}
	return env
}
