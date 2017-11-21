package executor

import (
	"os"
	"path/filepath"
	"testing"

	mesos "github.com/mesos/mesos-go/api/v1/lib"
	"github.com/stretchr/testify/assert"
)

func TestIfNewCancellableCommandReturnsCommand(t *testing.T) {
	os.Environ()
	commandInfo := newCommandInfo("./sleep 100", "ignored", false, []string{"ignored"}, map[string]string{"one": "1", "two": "2"})
	command, err := NewCommand(commandInfo, nil)
	cmd := command.(*cancellableCommand).cmd

	assert.NoError(t, err)
	assert.Equal(t, []string{"sh", "-c", "./sleep 100"}, cmd.Args)
	assert.Equal(t, filepath.Base(cmd.Path), "sh")
	assert.Contains(t, cmd.Env, "one=1")
	assert.Contains(t, cmd.Env, "two=2")
	assert.True(t, cmd.SysProcAttr.Setpgid, "should have pgid flag set to true")
}

func newCommandInfo(command, user string, shell bool, args []string, environment map[string]string) mesos.CommandInfo {
	env := mesos.Environment{}
	for key, value := range environment {
		// Go uses a copy of the value instead of the value itself within a range clause.
		k := key
		v := value
		env.Variables = append(env.Variables, mesos.Environment_Variable{Name: k, Value: v})
	}
	return mesos.CommandInfo{
		URIs:        nil,
		Environment: &env,
		Shell:       &shell,
		Value:       &command,
		Arguments:   args,
		User:        &user,
	}
}
