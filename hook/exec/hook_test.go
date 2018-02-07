package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/allegro/mesos-executor/hook"
)

func TestIfFailsToRunInvalidCommand(t *testing.T) {
	h := NewHook(HookCommand(hook.AfterTaskHealthyEvent, "invalid"))

	_, err := h.HandleEvent(hook.Event{Type: hook.AfterTaskHealthyEvent})

	assert.Error(t, err)
}

func TestIfRunsValidCommand(t *testing.T) {
	h := NewHook(HookCommand(hook.AfterTaskHealthyEvent, "echo", "test"))

	_, err := h.HandleEvent(hook.Event{Type: hook.AfterTaskHealthyEvent})

	assert.NoError(t, err)
}
