package exec_test

import (
	"log"
	"os"

	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/hook/exec"
)

func ExampleNewHook() {
	os.Setenv("TEST", "test")
	defer os.Unsetenv("TEST")

	h := exec.NewHook(exec.HookCommand(hook.AfterTaskHealthyEvent, "sh", "-c", "echo $TEST"))
	if _, err := h.HandleEvent(hook.Event{Type: hook.AfterTaskHealthyEvent}); err != nil {
		log.Fatal(err)
	}

	// Output: test
}
