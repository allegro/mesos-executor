package exec

import (
	"os"
	"os/exec"

	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor/hook"
)

// Hook is an executor hook implementation that will call defined external commands
// on specified hook events.
type Hook struct {
	commands map[hook.EventType]*exec.Cmd
}

// HandleEvent calls configured external command (if it is specified) for given
// hook event.
func (h *Hook) HandleEvent(event hook.Event) (hook.Env, error) {
	if cmd, ok := h.commands[event.Type]; ok {
		log.WithField("path", cmd.Path).WithField("args", cmd.Args).Info("Running hook command")
		return nil, cmd.Run()
	}
	log.Debugf("Received unsupported event type %s - ignoring", event.Type)
	return nil, nil // ignore unsupported events
}

// NewHook creates new exec hook with specified commands.
func NewHook(commands ...func(*Hook)) hook.Hook {
	h := &Hook{
		commands: make(map[hook.EventType]*exec.Cmd),
	}
	for _, command := range commands {
		command(h)
	}
	return h
}

// HookCommand sets a new command that will be run on specified event type. Only
// one command can be configured for each event type.
func HookCommand(eventType hook.EventType, name string, arg ...string) func(*Hook) {
	return func(h *Hook) {
		cmd := exec.Command(name, arg...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		h.commands[eventType] = cmd
	}
}
