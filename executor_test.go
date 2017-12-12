package executor

import (
	"context"
	"crypto/x509"
	"errors"
	"testing"
	"time"

	mesos "github.com/mesos/mesos-go/api/v1/lib"
	"github.com/mesos/mesos-go/api/v1/lib/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/state"
)

const infiniteCommand = "while :; do sleep 1; done"

const shortCommand = "sleep 1"

func TestIfLaunchesCommandAndSendsStateUpdates(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	stateUpdater := new(mockUpdater)
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_STARTING).Once()
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_RUNNING).Once()
	stateUpdater.On("UpdateWithOptions",
		mock.AnythingOfType("mesos.TaskID"),
		mesos.TASK_KILLED,
		mock.AnythingOfType("state.OptionalInfo")).Once()

	exec := new(Executor)
	exec.events = make(chan Event)
	exec.context = ctx
	exec.contextCancel = ctxCancel
	exec.stateUpdater = stateUpdater
	go exec.taskEventLoop()

	err := exec.handleMesosEvent(launchEventWithCommand(infiniteCommand))
	assert.NoError(t, err)
	err = exec.handleMesosEvent(killEvent())
	assert.NoError(t, err)

	<-exec.context.Done()
	assert.Equal(t, context.Canceled, exec.context.Err())
	stateUpdater.AssertExpectations(t)
}

func TestIfLaunchesCommandAndSendsStateUpdatesWhenTaskRequireCertButNoCertIsGiven(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	stateUpdater := new(mockUpdater)
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_STARTING).Once()
	stateUpdater.On("UpdateWithOptions",
		mock.AnythingOfType("mesos.TaskID"),
		mesos.TASK_FAILED,
		mock.MatchedBy(func(info state.OptionalInfo) bool {
			return "Cannot launch task: problem with certificate: missing certificate" == *info.Message
		})).Once()

	exec := new(Executor)
	exec.events = make(chan Event)
	exec.context = ctx
	exec.contextCancel = ctxCancel
	exec.stateUpdater = stateUpdater
	go exec.taskEventLoop()

	launchEvent := launchEventWithCommand(infiniteCommand)
	value := "true"
	launchEvent.Launch.Task.Labels = &mesos.Labels{
		Labels: []mesos.Label{{Key: "validate-certificate", Value: &value}}}

	exec.handleMesosEvent(launchEvent)

	// Wait for executor to finish
	<-exec.context.Done()

	stateUpdater.AssertExpectations(t)
}

func TestIfTaskKillingStateSentWhenFrameworkHasRequiredCapability(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	stateUpdater := new(mockUpdater)
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_STARTING).Once()
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_RUNNING).Once()
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_KILLING).Once()
	stateUpdater.On("UpdateWithOptions",
		mock.AnythingOfType("mesos.TaskID"),
		mesos.TASK_KILLED,
		mock.AnythingOfType("state.OptionalInfo")).Once()

	exec := new(Executor)
	exec.events = make(chan Event)
	exec.context = ctx
	exec.contextCancel = ctxCancel
	exec.stateUpdater = stateUpdater
	go exec.taskEventLoop()

	// ensure we get framework with required capabilities
	subErr := exec.handleMesosEvent(executor.Event{
		Type: executor.Event_SUBSCRIBED.Enum(),
		Subscribed: &executor.Event_Subscribed{
			FrameworkInfo: mesos.FrameworkInfo{
				Capabilities: []mesos.FrameworkInfo_Capability{
					{
						Type: mesos.FrameworkInfo_Capability_TASK_KILLING_STATE,
					},
				},
			},
		},
	})
	require.NoError(t, subErr)

	// launch task
	launchErr := exec.handleMesosEvent(launchEventWithCommand(infiniteCommand))
	require.NoError(t, launchErr)

	// kill task
	killErr := exec.handleMesosEvent(killEvent())
	assert.NoError(t, killErr)

	<-exec.context.Done()
	assert.Equal(t, context.Canceled, exec.context.Err())
	stateUpdater.AssertExpectations(t)
}

func TestIfTaskKillingStateNotSentWhenFrameworkDoesNotHaveRequiredCapability(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	stateUpdater := new(mockUpdater)
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_STARTING).Once()
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_RUNNING).Once()
	stateUpdater.On("UpdateWithOptions",
		mock.AnythingOfType("mesos.TaskID"),
		mesos.TASK_KILLED,
		mock.AnythingOfType("state.OptionalInfo")).Once()

	exec := new(Executor)
	exec.events = make(chan Event)
	exec.context = ctx
	exec.contextCancel = ctxCancel
	exec.stateUpdater = stateUpdater
	go exec.taskEventLoop()

	// ensure we get framework with no capabilities
	subErr := exec.handleMesosEvent(executor.Event{Type: executor.Event_SUBSCRIBED.Enum(), Subscribed: &executor.Event_Subscribed{}})
	require.NoError(t, subErr)

	// launch task
	launchErr := exec.handleMesosEvent(launchEventWithCommand(infiniteCommand))
	require.NoError(t, launchErr)

	// kill task
	killErr := exec.handleMesosEvent(killEvent())
	assert.NoError(t, killErr)

	<-exec.context.Done()
	assert.Equal(t, context.Canceled, exec.context.Err())
	stateUpdater.AssertExpectations(t)
}

func TestIfFiresAfterTaskHealthyOnlyOnFirstHealthyEvent(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	stateUpdater := new(mockUpdater)
	stateUpdater.On("UpdateWithOptions",
		mock.AnythingOfType("mesos.TaskID"),
		mesos.TASK_RUNNING,
		mock.AnythingOfType("state.OptionalInfo")).Twice()

	exec := new(Executor)
	exec.events = make(chan Event)
	exec.context = ctx
	exec.contextCancel = ctxCancel
	exec.stateUpdater = stateUpdater
	go exec.taskEventLoop()

	mockedHook := new(mockHook)
	mockedHook.On("HandleEvent", mock.MatchedBy(func(event hook.Event) bool {
		return event.Type == hook.AfterTaskHealthyEvent
	})).Return(hook.Env{}, nil).Once()
	exec.hookManager.Hooks = append(exec.hookManager.Hooks, mockedHook)

	exec.events <- Event{
		Type: Healthy,
	}
	exec.events <- Event{
		Type: Healthy,
	}

	mockedHook.AssertExpectations(t)
}

func TestIfStopsAfterTaskHealthyEventHookFail(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	stateUpdater := new(mockUpdater)
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_STARTING).Once()
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_RUNNING).Once()
	stateUpdater.On("UpdateWithOptions",
		mock.AnythingOfType("mesos.TaskID"),
		mesos.TASK_FAILED,
		mock.AnythingOfType("state.OptionalInfo")).Once()

	mockedHook := new(mockHook)
	mockedHook.On("HandleEvent", mock.MatchedBy(func(event hook.Event) bool {
		return event.Type == hook.BeforeTaskStartEvent
	})).Return(hook.Env{}, nil).Once()
	mockedHook.On("HandleEvent", mock.MatchedBy(func(event hook.Event) bool {
		return event.Type == hook.AfterTaskHealthyEvent
	})).Return(hook.Env{}, errors.New("error")).Once()
	mockedHook.On("HandleEvent", mock.MatchedBy(func(event hook.Event) bool {
		return event.Type == hook.BeforeTerminateEvent
	})).Return(hook.Env{}, nil).Once()

	exec := new(Executor)
	exec.events = make(chan Event)
	exec.context = ctx
	exec.contextCancel = ctxCancel
	exec.hookManager.Hooks = []hook.Hook{mockedHook}
	exec.stateUpdater = stateUpdater
	go exec.taskEventLoop()

	launchErr := exec.handleMesosEvent(launchEventWithCommand(infiniteCommand))
	require.NoError(t, launchErr)

	exec.events <- Event{
		Type: Healthy,
	}

	<-exec.context.Done()
	mockedHook.AssertExpectations(t)
}

func TestIfHookCalledAfterTaskExits(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	stateUpdater := new(mockUpdater)
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_STARTING).Once()
	stateUpdater.On("Update", mock.AnythingOfType("mesos.TaskID"), mesos.TASK_RUNNING).Once()
	stateUpdater.On("UpdateWithOptions",
		mock.AnythingOfType("mesos.TaskID"),
		mesos.TASK_RUNNING,
		mock.AnythingOfType("state.OptionalInfo")).Once()
	stateUpdater.On("UpdateWithOptions",
		mock.AnythingOfType("mesos.TaskID"),
		mesos.TASK_FAILED,
		mock.AnythingOfType("state.OptionalInfo")).Once()

	mockedHook := new(mockHook)
	mockedHook.On("HandleEvent", mock.MatchedBy(func(event hook.Event) bool {
		return event.Type == hook.BeforeTaskStartEvent
	})).Return(hook.Env{}, nil).Once()
	mockedHook.On("HandleEvent", mock.MatchedBy(func(event hook.Event) bool {
		return event.Type == hook.AfterTaskHealthyEvent
	})).Return(hook.Env{}, nil).Once()
	mockedHook.On("HandleEvent", mock.MatchedBy(func(event hook.Event) bool {
		return event.Type == hook.BeforeTerminateEvent
	})).Return(hook.Env{}, nil).Once()

	exec := new(Executor)
	exec.events = make(chan Event)
	exec.context = ctx
	exec.contextCancel = ctxCancel
	exec.hookManager.Hooks = []hook.Hook{mockedHook}
	exec.stateUpdater = stateUpdater
	go exec.taskEventLoop()

	launchErr := exec.handleMesosEvent(launchEventWithCommand(shortCommand))
	require.NoError(t, launchErr)

	exec.events <- Event{
		Type: Healthy,
	}

	<-exec.context.Done()
	mockedHook.AssertExpectations(t)
}

func TestCertificateCheckReturnErrorIfKillWillBeScheduledInThePast(t *testing.T) {
	fakeCert := &x509.Certificate{NotAfter: time.Time{}}

	clock := new(mockClock)
	clock.On("Until", mock.MatchedBy(func(t time.Time) bool {
		return t == fakeCert.NotAfter
	})).Return(time.Nanosecond).Once()

	random := new(mockRandom)
	random.On("Duration", mock.MatchedBy(func(max time.Duration) bool {
		return max == time.Hour
	})).Return(time.Second).Once()

	exec := &Executor{
		events: make(chan Event),
		clock:  clock,
		random: random,
		config: Config{RandomExpirationRange: time.Hour},
	}

	err := exec.checkCert(fakeCert)

	assert.EqualError(t, err, "certificate valid period <= 0 - certificate invalid after 0001-01-01 00:00:00 +0000 UTC")
	random.AssertExpectations(t)
	clock.AssertExpectations(t)
}

func TestCertificateCheckScheduleTaskKillBeforeCertificateExpires(t *testing.T) {
	fakeCert := &x509.Certificate{NotAfter: time.Time{}}

	clock := new(mockClock)
	clock.On("Until", mock.MatchedBy(func(t time.Time) bool {
		return t == fakeCert.NotAfter
	})).Return(time.Microsecond).Once()

	random := new(mockRandom)
	random.On("Duration", mock.MatchedBy(func(max time.Duration) bool {
		return max == time.Hour
	})).Return(time.Nanosecond).Once()

	exec := &Executor{
		events: make(chan Event),
		clock:  clock,
		random: random,
		config: Config{RandomExpirationRange: time.Hour},
	}

	err := exec.checkCert(fakeCert)

	assert.NoError(t, err)
	random.AssertExpectations(t)
	clock.AssertExpectations(t)

	time.Sleep(time.Nanosecond)
	event := <-exec.events

	assert.Equal(t, Event{Type: FailedDueToExpiredCertificate, Message: "Certificate expired"}, event)
}

type mockClock struct {
	mock.Mock
}

func (m *mockClock) Until(t time.Time) time.Duration {
	arg := m.Called(t)
	return arg.Get(0).(time.Duration)
}

type mockRandom struct {
	mock.Mock
}

func (m *mockRandom) Duration(max time.Duration) time.Duration {
	arg := m.Called(max)
	return arg.Get(0).(time.Duration)
}

type mockHook struct {
	mock.Mock
}

func (m *mockHook) HandleEvent(event hook.Event) (hook.Env, error) {
	arg := m.Called(event)
	return arg.Get(0).(hook.Env), arg.Error(1)
}

type mockUpdater struct {
	mock.Mock
}

func (u *mockUpdater) Acknowledge(id []byte) {
	u.Called(id)
}

func (u *mockUpdater) GetUnacknowledged() []executor.Call_Update {
	args := u.Called()
	return args.Get(0).([]executor.Call_Update)
}

func (u *mockUpdater) Update(taskID mesos.TaskID, state mesos.TaskState) {
	u.Called(taskID, state)
}

func (u *mockUpdater) UpdateWithOptions(taskID mesos.TaskID, state mesos.TaskState, opt state.OptionalInfo) {
	u.Called(taskID, state, opt)
}

func (u *mockUpdater) Wait(timeout time.Duration) error {
	u.Called(timeout)
	return nil
}

func launchEventWithCommand(command string) executor.Event {
	return executor.Event{
		Type: executor.Event_LAUNCH.Enum(),
		Launch: &executor.Event_Launch{
			Task: mesos.TaskInfo{
				Executor: &mesos.ExecutorInfo{
					Command: mesos.CommandInfo{
						Value: &command}}}}}
}

func killEvent() executor.Event {
	return executor.Event{Type: executor.Event_KILL.Enum(), Kill: &executor.Event_Kill{}}
}
