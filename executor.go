package executor

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	mesos "github.com/mesos/mesos-go/api/v1/lib"
	"github.com/mesos/mesos-go/api/v1/lib/backoff"
	"github.com/mesos/mesos-go/api/v1/lib/encoding"
	"github.com/mesos/mesos-go/api/v1/lib/executor"
	"github.com/mesos/mesos-go/api/v1/lib/executor/calls"
	"github.com/mesos/mesos-go/api/v1/lib/executor/config"
	"github.com/mesos/mesos-go/api/v1/lib/httpcli"

	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/mesosutils"
	"github.com/allegro/mesos-executor/state"
)

// Config settable from the environment
type Config struct {
	// Sets logging level to `debug` when true, `info` otherwise
	Debug bool `default:"false" split_words:"true"`
	// Mesos API path
	APIPath string `default:"/api/v1/executor" split_words:"true"`
	// Delay between sending TERM and KILL signals
	KillPolicyGracePeriod time.Duration `default:"5s" split_words:"true"`
	// Timeout for communication with Mesos
	HTTPTimeout time.Duration `default:"10s" split_words:"true"`
	// Number of state messages to keep in buffer
	StateUpdateBufferSize int `default:"1024" split_words:"true"`
	// Timeout for attempts to send messages in buffer
	StateUpdateWaitTimeout time.Duration `default:"5s" split_words:"true"`

	// Mesos framework configuration
	MesosConfig config.Config `ignore:"true"`

	// SentryDSN is an address used for sending logs to Sentry
	SentryDSN string `split_words:"true"`

	// Range in which certificate will be considered as expired. Used to
	// prevent shutdown of all tasks at once.
	RandomExpirationRange time.Duration `default:"3h" split_words:"true"`
}

var errMustAbort = errors.New("received abort signal from mesos, will attempt to re-subscribe")

// Executor is responsible for launching and monitoring single Mesos task.
type Executor struct {
	config        Config
	context       context.Context
	contextCancel context.CancelFunc
	framework     mesos.FrameworkInfo
	hookManager   hook.Manager
	stateUpdater  state.Updater
	events        chan Event
	clock         clock
	random        random
}

// Event is an internal executor event that triggers specific actions driven
// by current state and Type.
type Event struct {
	Type EventType
	// Message store the human readable information about
	// current event. For example reason of the event or
	// additional debug message.
	Message string

	//TODO(janisz): Create domain objects for this.
	subscribed executor.Event_Subscribed
	launch     executor.Event_Launch
}

// EventType defines type of the Event.
type EventType int

const (
	// Healthy means task health check passed and task healthy.
	Healthy EventType = iota
	// Unhealthy means task health check failed. Fail reason should be
	// passed in Event Message field.
	Unhealthy
	// FailedDueToUnhealthy means task health check failed and task should be killed
	// because it's unhealthy for longer period of time. Fail reason should be
	// passed in Event Message field.
	FailedDueToUnhealthy
	// FailedDueToExpiredCertificate means task certificate expired (or will expire soon)
	// and task should be killed because it can't work with invalid certificate.
	FailedDueToExpiredCertificate

	// CommandExited means command has exited. Message should contains information
	// about exit code.
	CommandExited

	// Kill means command should be killed and executor exit.
	Kill

	// Subscribed means executor attach to mesos Agent.
	Subscribed
	// Launch means executor should start a task.
	Launch
)

// NewExecutor creates new instance of executor configured with by `cfg` with hooks
func NewExecutor(cfg Config, hooks ...hook.Hook) *Executor {

	log.Info("Initializing executor with following configuration:")
	log.Infof("AgentEndpoint               = %s", cfg.MesosConfig.AgentEndpoint)
	log.Infof("Checkpoint                  = %t", cfg.MesosConfig.Checkpoint)
	log.Infof("Directory                   = %s", cfg.MesosConfig.Directory)
	log.Infof("ExecutorID                  = %s", cfg.MesosConfig.ExecutorID)
	log.Infof("ExecutorShutdownGracePeriod = %s", cfg.MesosConfig.ExecutorShutdownGracePeriod)
	log.Infof("FrameworkID                 = %s", cfg.MesosConfig.FrameworkID)
	log.Infof("RecoveryTimeout             = %s", cfg.MesosConfig.RecoveryTimeout)
	log.Infof("SubscriptionBackoffMax      = %s", cfg.MesosConfig.SubscriptionBackoffMax)
	log.Infof("APIPath                     = %s", cfg.APIPath)

	ctx, ctxCancel := context.WithCancel(context.Background())
	return &Executor{
		config:        cfg,
		context:       ctx,
		contextCancel: ctxCancel,
		events:        make(chan Event),
		hookManager:   hook.Manager{Hooks: hooks},
		stateUpdater:  state.BufferedUpdater(cfg.MesosConfig, cfg.StateUpdateBufferSize),
		clock:         systemClock{},
		random:        newRandom(),
	}
}

// Start registers executor in Mesos agent and waits for events from it.
func (e *Executor) Start() error {

	go e.taskEventLoop()

	callOptions := executor.CallOptions{
		calls.Executor(e.config.MesosConfig.ExecutorID),
		calls.Framework(e.config.MesosConfig.FrameworkID),
	}

	apiURL := url.URL{
		Scheme: "http",
		Host:   e.config.MesosConfig.AgentEndpoint,
		Path:   e.config.APIPath,
	}

	httpClient := httpcli.New(
		httpcli.Endpoint(apiURL.String()),
		httpcli.Codec(&encoding.ProtobufCodec),
		httpcli.Do(httpcli.With(httpcli.Timeout(e.config.HTTPTimeout))),
	)

	shouldConnect := backoff.Notifier(time.Second, e.config.MesosConfig.SubscriptionBackoffMax, nil)
	recoveryTimeout := time.NewTimer(e.config.MesosConfig.RecoveryTimeout)

SUBSCRIBE_LOOP:
	for {
		select {
		case <-recoveryTimeout.C:
			return fmt.Errorf("Failed to re-establish subscription with agent within %v, aborting", e.config.MesosConfig.RecoveryTimeout)
		case <-e.context.Done():
			break SUBSCRIBE_LOOP
		case <-shouldConnect:
			subscribe := calls.Subscribe(nil, e.stateUpdater.GetUnacknowledged()).With(callOptions...)
			resp, err := httpClient.Do(subscribe, httpcli.Close(true))
			if err == nil {
				err = e.eventLoop(resp.Decoder())
				e.handleConnError(err)
				if !recoveryTimeout.Stop() {
					<-recoveryTimeout.C
				}
				recoveryTimeout.Reset(e.config.MesosConfig.RecoveryTimeout)

				if closeErr := resp.Close(); closeErr != nil {
					log.WithError(closeErr).Warn("Error during agent response closing")
				}
			} else {
				e.handleConnError(err)
			}
		}
	}

	if err := e.stateUpdater.Wait(e.config.StateUpdateWaitTimeout); err != nil { // try to send remaining state updates
		log.WithError(err).Error("Unable to send remaining state updates to Mesos agent")
	}

	return nil
}

func (e *Executor) eventLoop(decoder encoding.Decoder) (err error) {
	for err == nil {
		select {
		case <-e.context.Done():
			return nil
		default:
			var event executor.Event
			if err = decoder.Invoke(&event); err == nil {
				err = e.handleMesosEvent(event)
			}
		}
	}
	return err
}

func (e *Executor) handleMesosEvent(event executor.Event) error {
	log.WithField("Type", event.Type).Info("Event received")

	switch event.GetType() {
	case executor.Event_SUBSCRIBED:
		e.events <- Event{Type: Subscribed, subscribed: *event.GetSubscribed()}
	case executor.Event_LAUNCH:
		e.events <- Event{Type: Launch, launch: *event.GetLaunch()}
	case executor.Event_KILL, executor.Event_SHUTDOWN:
		e.events <- Event{Type: Kill}
	case executor.Event_ERROR:
		return errMustAbort
	case executor.Event_ACKNOWLEDGED:
		e.stateUpdater.Acknowledge(event.GetAcknowledged().GetUUID())
	default:
		log.WithField("Type", event.Type).Warn("Unknown event type. Event: %s", event.GoString())
	}

	return nil
}

func (e *Executor) handleConnError(err error) {
	if err == io.EOF {
		log.Info("Disconnected from Mesos agent")
	} else if err != nil {
		log.WithError(err).Warn("Mesos agent connection error")
	}
}

// taskEventLoop is responsible for receiving updates about task/command/health and handle them.
func (e *Executor) taskEventLoop() {
	defer e.contextCancel()

	var taskInfo mesos.TaskInfo
	var cmd Command

	fireHealthyHook := true

	for event := range e.events {
		switch event.Type {
		case Subscribed:
			e.framework = event.subscribed.GetFrameworkInfo()
		case Launch:
			taskInfo = event.launch.GetTask()
			var err error
			cmd, err = e.launchTask(taskInfo)
			if err != nil {
				msg := fmt.Sprintf("Canot launch task: %s", err)
				e.stateUpdater.UpdateWithOptions(taskInfo.GetTaskID(), mesos.TASK_FAILED, state.OptionalInfo{Message: &msg})
				return
			}
		case Healthy:
			if fireHealthyHook {
				fireHealthyHook = false
				event := hook.Event{
					Type:     hook.AfterTaskHealthyEvent,
					TaskInfo: mesosutils.TaskInfo{TaskInfo: taskInfo},
				}
				if err := e.hookManager.HandleEvent(event, false); err != nil { // do not ignore errors here, so we will not have an incorrectly configured service
					log.WithError(err).Errorf("Error calling after task healthy hooks. Stopping the command.")
					msg := fmt.Sprintf("Error calling after task healthy hooks: %s", err)
					e.shutDown(taskInfo, cmd)
					e.stateUpdater.UpdateWithOptions(taskInfo.GetTaskID(), mesos.TASK_FAILED, state.OptionalInfo{Message: &msg})
					return
				}
			}

			healthy := true
			e.stateUpdater.UpdateWithOptions(taskInfo.GetTaskID(), mesos.TASK_RUNNING, state.OptionalInfo{Healthy: &healthy})
		case Unhealthy:
			unhealthy := false
			e.stateUpdater.UpdateWithOptions(taskInfo.GetTaskID(), mesos.TASK_RUNNING, state.OptionalInfo{Healthy: &unhealthy, Message: &event.Message})
		case FailedDueToUnhealthy, FailedDueToExpiredCertificate:
			unhealthy := false
			info := state.OptionalInfo{Healthy: &unhealthy, Message: &event.Message}
			e.stateUpdater.UpdateWithOptions(taskInfo.GetTaskID(), mesos.TASK_RUNNING, info)
			log.WithFields(log.Fields{"TaskID": taskInfo.GetTaskID(), "Reason": event.Message}).Info("Killing task")
			e.shutDown(taskInfo, cmd)
			e.stateUpdater.UpdateWithOptions(taskInfo.GetTaskID(), mesos.TASK_FAILED, info)
			return
		case CommandExited:
			e.shutDown(taskInfo, cmd)
			e.stateUpdater.UpdateWithOptions(taskInfo.GetTaskID(), mesos.TASK_FAILED, state.OptionalInfo{Message: &event.Message})
			return
		case Kill:
			e.shutDown(taskInfo, cmd)
			message := "Task killed due to receiving an event from Mesos agent"
			e.stateUpdater.UpdateWithOptions(
				taskInfo.GetTaskID(),
				mesos.TASK_KILLED,
				state.OptionalInfo{
					Message: &message,
				},
			)
			return
		}
	}
}

func (e *Executor) launchTask(taskInfo mesos.TaskInfo) (Command, error) {
	commandInfo := taskInfo.GetExecutor().GetCommand()
	e.stateUpdater.Update(taskInfo.GetTaskID(), mesos.TASK_STARTING)
	prepareCommandInfo(&commandInfo)

	env := os.Environ()

	validateCertificate := mesosutils.TaskInfo{TaskInfo: taskInfo}.GetLabelValue("validate-certificate")
	if validateCertificate == "true" {
		if certificate, err := GetCertFromEnvVariables(env); err != nil {
			return nil, fmt.Errorf("Problem with certificate: %s", err)
		} else if err := e.checkCert(certificate); err != nil {
			return nil, fmt.Errorf("Problem with certificate: %s", err)
		}
	}

	cmd, err := NewCommand(commandInfo, env)
	if err != nil {
		return nil, fmt.Errorf("Cannot create command: %s", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("Cannot start command: %s", err)
	}

	go taskExitToEvent(cmd.Wait(), e.events)

	e.stateUpdater.Update(taskInfo.GetTaskID(), mesos.TASK_RUNNING)

	if taskInfo.GetHealthCheck() != nil {
		DoHealthChecks(*taskInfo.GetHealthCheck(), e.events)
	}

	return cmd, nil
}

func (e *Executor) checkCert(cert *x509.Certificate) error {
	certDuration := e.clock.Until(cert.NotAfter) - e.random.Duration(e.config.RandomExpirationRange)
	if certDuration <= 0 {
		return fmt.Errorf("Certificate valid period <= 0. Certificate invalid after %s", cert.NotAfter)
	}

	log.WithField("CertificateExpireDate", cert.NotAfter).Infof(
		"Schedule task kill in %s", certDuration)
	time.AfterFunc(certDuration, func() {
		e.events <- Event{Type: FailedDueToExpiredCertificate, Message: "Certificate expired"}
	})

	return nil
}

func (e *Executor) shutDown(taskInfo mesos.TaskInfo, cmd Command) {
	for _, capability := range e.framework.GetCapabilities() {
		if capability.GetType() == mesos.FrameworkInfo_Capability_TASK_KILLING_STATE {
			e.stateUpdater.Update(taskInfo.GetTaskID(), mesos.TASK_KILLING)
		}
	}

	gracePeriod := e.config.KillPolicyGracePeriod
	if ns := taskInfo.GetKillPolicy().GetGracePeriod().GetNanoseconds(); ns > 0 {
		gracePeriod = time.Duration(ns)
	}
	beforeTerminateEvent := hook.Event{
		Type:     hook.BeforeTerminateEvent,
		TaskInfo: mesosutils.TaskInfo{TaskInfo: taskInfo},
	}
	_ = e.hookManager.HandleEvent(beforeTerminateEvent, true) // ignore errors here, so every hook will have a chance to be called
	cmd.Stop(gracePeriod)                                     // blocking call
}

func taskExitToEvent(exitStateChan <-chan TaskExitState, events chan<- Event) {
	exitState := <-exitStateChan
	switch exitState.Code {
	case FailedCode:
		events <- Event{Type: CommandExited, Message: fmt.Sprintf("Task exited with an error: %s", exitState.Err.Error())}
	case SuccessCode:
		events <- Event{Type: CommandExited, Message: "Task exited with success (zero) exit code"}
	}
}

// Hack: For Marathon #4952
func prepareCommandInfo(commandInfo *mesos.CommandInfo) {
	marathonPrefix := fmt.Sprintf("chmod ug+rx '%s' && exec '%s' ", os.Args[0], os.Args[0])
	commandLine := strings.TrimPrefix(commandInfo.GetValue(), marathonPrefix)
	log.Debugf("Replacing prefix ”%s” from ”%s” results with ”%s”", marathonPrefix, commandInfo.GetValue(), commandLine)
	commandInfo.Value = &commandLine
}
