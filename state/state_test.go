package state

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	mesos "github.com/mesos/mesos-go/api/v1/lib"
	"github.com/mesos/mesos-go/api/v1/lib/executor/config"
	"github.com/stretchr/testify/assert"
)

func TestIfSendsBufferedStateUpdatesOnExit(t *testing.T) {
	const updatesCount = 5
	calls := make(chan bool, updatesCount)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)

		assert.True(t, bytes.Contains(body, []byte("executorID")))
		assert.True(t, bytes.Contains(body, []byte("frameworkID")))

		rw.Header().Add("Content-Type", "application/x-protobuf")
		rw.WriteHeader(http.StatusOK)

		calls <- true
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	cfg := config.Config{
		AgentEndpoint: fmt.Sprintf("%s:%s", url.Hostname(), url.Port()),
		ExecutorID:    "executorID",
		FrameworkID:   "frameworkID",
	}
	updater := BufferedUpdater(cfg, updatesCount) // ensure async Update call

	// fill buffer with some data
	for i := 0; i < updatesCount; i++ {
		updater.Update(mesos.TaskID{Value: "TaskID"}, mesos.TASK_RUNNING)
	}

	// check if server was called
	for i := 0; i < updatesCount; i++ {
		called := <-calls
		assert.True(t, called)
	}

	// all updates should be unacknowledeged
	assert.Len(t, updater.GetUnacknowledged(), updatesCount)

	// acknowledge all updates
	for _, ack := range updater.GetUnacknowledged() {
		updater.Acknowledge(ack.Status.GetUUID())
	}

	err := updater.Wait(time.Second)

	assert.NoError(t, err)
}

func TestIfSendsUpdatesToMesosAgent(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)

		assert.True(t, bytes.Contains(body, []byte("executorID")))
		assert.True(t, bytes.Contains(body, []byte("frameworkID")))

		rw.Header().Add("Content-Type", "application/x-protobuf")
		rw.WriteHeader(http.StatusOK)
		done <- struct{}{}
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	cfg := config.Config{
		AgentEndpoint: fmt.Sprintf("%s:%s", url.Hostname(), url.Port()),
		ExecutorID:    "executorID",
		FrameworkID:   "frameworkID",
	}
	updater := BufferedUpdater(cfg, 0) // force sync Update call

	updater.Update(mesos.TaskID{Value: "TaskID"}, mesos.TASK_RUNNING)
	<-done // wait for server to be caled
}

func TestIfSendsUpdatesWithMessageToMesosAgent(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)

		assert.True(t, bytes.Contains(body, []byte("executorID")))
		assert.True(t, bytes.Contains(body, []byte("frameworkID")))
		assert.True(t, bytes.Contains(body, []byte("test-message")))

		rw.Header().Add("Content-Type", "application/x-protobuf")
		rw.WriteHeader(http.StatusOK)
		done <- struct{}{}
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	cfg := config.Config{
		AgentEndpoint: fmt.Sprintf("%s:%s", url.Hostname(), url.Port()),
		ExecutorID:    "executorID",
		FrameworkID:   "frameworkID",
	}
	updater := BufferedUpdater(cfg, 0) // force sync Update call

	testMessage := "test-message"
	updater.UpdateWithOptions(mesos.TaskID{Value: "TaskID"}, mesos.TASK_RUNNING, OptionalInfo{Message: &testMessage})
	<-done // wait for server to be called
}
