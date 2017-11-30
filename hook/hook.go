//go:generate stringer -type=EventType

package hook

import "github.com/allegro/mesos-executor/mesosutils"

// EventType represents task lifecycle event type.
type EventType int

const (
	// BeforeTaskStartEvent is an event type that occurs right before task is
	// started. It is not guaranteed to occur in task lifecycle.
	BeforeTaskStartEvent EventType = iota
	// AfterTaskHealthyEvent is an event type that occurs right after first successful
	// task health check pass.
	AfterTaskHealthyEvent
	// BeforeTerminateEvent is an event type that occurs right before task is terminated.
	// It is guaranteed to occur in task lifecycle and to be last event received.
	BeforeTerminateEvent
)

// Event is a container type for various event specific data.
type Event struct {
	Type     EventType
	TaskInfo mesosutils.TaskInfo
}

// Env is a container for os.Environ style list of combined environment variable strings.
type Env []string

// Hook is an interface for various executor extensions, that can add some actions
// during task lifecycle events.
type Hook interface {
	// HandleEvent is called when any of defined executor task events occurs.
	// Received events may be handled in any way, but hook should ignore unknown or
	// unsupported ones. Call to this function will block executor process until
	// it returns. Order of received event types is undefined.
	HandleEvent(Event) (Env, error)
}
