package state

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	mesos "github.com/mesos/mesos-go/api/v1/lib"
	"github.com/mesos/mesos-go/api/v1/lib/encoding"
	"github.com/mesos/mesos-go/api/v1/lib/executor"
	"github.com/mesos/mesos-go/api/v1/lib/executor/calls"
	"github.com/mesos/mesos-go/api/v1/lib/executor/config"
	"github.com/mesos/mesos-go/api/v1/lib/httpcli"
	"github.com/pborman/uuid"
)

const (
	apiPath = "/api/v1/executor"

	// httpTimeout is a connection and keep-alive timeout used by HTTP client
	httpTimeout = 10 * time.Second
)

// OptionalInfo contains optional info that could be attached to Task State update.
type OptionalInfo struct {
	// Message is additional message that will be added to Task State. Use nil for empty.
	Message *string
	// Healthy indicates if task is healthy (true) or not (false) for unknown pass nil.
	Healthy *bool
}

// Updater is an interface for types responsible for updating task status in
// Mesos agent. Implementation should handle all the retry logic when an agent is
// offline.
type Updater interface {
	// Update sends task state update to Mesos agent. It should be a non-blocking
	// call.
	Update(mesos.TaskID, mesos.TaskState)

	// UpdateWithMessage sends task state update with optional fields to Mesos agent. It
	// should be a non-blocking call.
	UpdateWithOptions(mesos.TaskID, mesos.TaskState, OptionalInfo)

	// Acknowledge marks task state update with matching uuid as acknowledged by Mesos agent.
	Acknowledge([]byte)

	// GetUnacknowledged returns slice of unacknowledged task statuses.
	GetUnacknowledged() []executor.Call_Update

	// Wait continues sending state updates to Mesos agent until all of them are
	// sent or given duration is exceeded.
	Wait(time.Duration) error
}

type bufferedUpdater struct {
	mutex         sync.RWMutex
	buffer        chan mesos.TaskStatus
	bufferSize    int
	callOptions   executor.CallOptions
	cfg           config.Config
	ctx           context.Context
	ctxCancel     context.CancelFunc
	httpClient    *httpcli.Client
	unAckStatuses map[string]mesos.TaskStatus
}

func (u *bufferedUpdater) Update(taskID mesos.TaskID, state mesos.TaskState) {
	u.update(taskID, state, nil, nil)
}

func (u *bufferedUpdater) UpdateWithOptions(taskID mesos.TaskID, state mesos.TaskState, opt OptionalInfo) {
	u.update(taskID, state, opt.Message, opt.Healthy)
}

func (u *bufferedUpdater) update(taskID mesos.TaskID, state mesos.TaskState, message *string, healthy *bool) {
	now := float64(time.Now().Unix())
	status := mesos.TaskStatus{
		TaskID:     taskID,
		Source:     mesos.SOURCE_EXECUTOR.Enum(),
		State:      &state,
		Message:    message,
		Healthy:    healthy,
		ExecutorID: &mesos.ExecutorID{Value: u.cfg.ExecutorID},
		Timestamp:  &now,
		UUID:       []byte(uuid.NewRandom()),
	}
	u.buffer <- status
}

func (u *bufferedUpdater) Acknowledge(id []byte) {
	uuidString := uuid.UUID(id).String()
	log.WithField("UUID", uuidString).Info("Mesos acknowledged status update")
	u.mutex.Lock()
	delete(u.unAckStatuses, uuid.UUID(id).String())
	u.mutex.Unlock()
}

func (u *bufferedUpdater) GetUnacknowledged() []executor.Call_Update {
	u.mutex.RLock()
	unacknowledged := make([]executor.Call_Update, 0, len(u.unAckStatuses))
	for _, taskStatus := range u.unAckStatuses {
		callUpdate := executor.Call_Update{Status: taskStatus}
		unacknowledged = append(unacknowledged, callUpdate)
	}
	u.mutex.RUnlock()
	return unacknowledged
}

func (u *bufferedUpdater) Wait(timeout time.Duration) error {
	defer u.ctxCancel()

	ticker := time.NewTicker(time.Second)
	start := time.Now()

	for range ticker.C {
		if len(u.buffer) == 0 && len(u.GetUnacknowledged()) == 0 {
			return nil
		} else if time.Since(start) >= timeout {
			return fmt.Errorf("Timeout during state update buffer cleaning, %d events remained, %d events unacknowledged",
				len(u.buffer), len(u.GetUnacknowledged()))
		}
	}

	return nil
}

func (u *bufferedUpdater) loop() {
	go func() {
		for {
			select {
			case status := <-u.buffer:
				stringUUID := uuid.UUID(status.GetUUID()).String()
				log.WithFields(log.Fields{
					"Type": status.GetState(),
					"UUID": stringUUID,
				}).Info("Sending task state update to Mesos agent")

				u.mutex.Lock()
				u.unAckStatuses[stringUUID] = status
				u.mutex.Unlock()

				if err := u.send(status); err != nil {
					log.WithError(err).Warnf("Error sending %s task state update, requeuing", status.GetState())
					u.buffer <- status
				}
			case <-u.ctx.Done():
				return
			}
		}
	}()
}

func (u *bufferedUpdater) send(status mesos.TaskStatus) error {
	update := calls.Update(status).With(u.callOptions...)
	response, err := u.httpClient.Do(update)

	if response != nil {
		if err = response.Close(); err != nil {
			log.WithError(err).Warn("Error closing response from Mesos agent during task state update")
		}
	}

	return err
}

// BufferedUpdater returns an updater implementation that keeps state updates
// in a buffered channel (to allow non-blocking calls to the Update function).
// It will be trying to send buffered state updates in a background goroutine
// until Wait is called.
func BufferedUpdater(cfg config.Config, bufferSize int) Updater {
	buffer := make(chan mesos.TaskStatus, bufferSize)
	callOptions := executor.CallOptions{
		calls.Executor(cfg.ExecutorID),
		calls.Framework(cfg.FrameworkID),
	}
	apiURL := url.URL{
		Scheme: "http",
		Host:   cfg.AgentEndpoint,
		Path:   apiPath,
	}
	httpClient := httpcli.New(
		httpcli.Endpoint(apiURL.String()),
		httpcli.Codec(&encoding.ProtobufCodec),
		httpcli.Do(httpcli.With(httpcli.Timeout(httpTimeout))),
	)
	ctx, ctxCancelFunc := context.WithCancel(context.Background())
	updater := &bufferedUpdater{
		buffer:        buffer,
		callOptions:   callOptions,
		cfg:           cfg,
		ctx:           ctx,
		ctxCancel:     ctxCancelFunc,
		httpClient:    httpClient,
		unAckStatuses: make(map[string]mesos.TaskStatus),
	}
	updater.loop()
	return updater
}
