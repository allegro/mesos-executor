package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mesos/mesos-go/api/v1/lib"
	"github.com/stretchr/testify/assert"
)

func TestIfNewCancellableCommandReturnsCommandWithoutExecutorEnv(t *testing.T) {
	os.Setenv("ALLEGRO_EXECUTOR_TEST_1", "x")
	os.Setenv("allegro_executor_TEST_2", "y")
	os.Setenv("TEST", "z")
	os.Setenv("_ALLEGRO_EXECUTOR_", "0")

	defer os.Unsetenv("ALLEGRO_EXECUTOR_TEST_1")
	defer os.Unsetenv("allegro_executor_TEST_2")
	defer os.Unsetenv("TEST")
	defer os.Unsetenv("_ALLEGRO_EXECUTOR_")

	commandInfo := newCommandInfo("./sleep 100", "ignored", false, []string{"ignored"})
	command, err := NewCommand(commandInfo, []string{"ALLEGRO_EXECUTOR_NOT_REMOVED=y", "SOME_ENV=x"})
	cmd := command.(*cancellableCommand).cmd

	assert.NoError(t, err)
	assert.Equal(t, []string{"sh", "-c", "./sleep 100"}, cmd.Args)
	assert.Equal(t, filepath.Base(cmd.Path), "sh")
	assert.True(t, cmd.SysProcAttr.Setpgid, "should have pgid flag set to true")

	assert.NotContains(t, cmd.Env, "ALLEGRO_EXECUTOR_TEST_1=x")

	for _, e := range []string{"allegro_executor_TEST_2=y", "TEST=z", "_ALLEGRO_EXECUTOR_=0", "ALLEGRO_EXECUTOR_NOT_REMOVED=y", "SOME_ENV=x"} {
		assert.Contains(t, cmd.Env, e)
	}
}

func newCommandInfo(command, user string, shell bool, args []string) mesos.CommandInfo {
	return mesos.CommandInfo{
		Shell:     &shell,
		Value:     &command,
		Arguments: args,
		User:      &user,
	}
}
