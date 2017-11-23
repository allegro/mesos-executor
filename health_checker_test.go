package executor

import (
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	mesos "github.com/mesos/mesos-go/api/v1/lib"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoHealthChecksShouldStartHealthCheckig(t *testing.T) {
	delay := time.Millisecond.Seconds()
	gracePeriod := 0.0
	interval := time.Millisecond.Seconds()

	// Create always failing health check.
	check := mesos.HealthCheck{
		GracePeriodSeconds: &gracePeriod,
		DelaySeconds:       &delay,
		IntervalSeconds:    &interval,
	}
	healthStates := make(chan Event)

	// Start health checking.
	DoHealthChecks(check, healthStates)

	// Wait for first health check result.
	select {
	case <-healthStates:
	case <-time.After(time.Second):
		t.Error("Health check state should come in configured timeout")
	}

	// Wait for next health check result.
	select {
	case <-healthStates:
	case <-time.After(time.Second):
		t.Error("Health check state should come in configured timeout")
	}
}

func TestHandleHealthResultsShouldProxyAllUnhealthyResultsAfterGracePeriod(t *testing.T) {
	healthResults := make(chan error)
	healthStates := make(chan Event)
	gracePeriod := 0.0
	go handleHealthResults(mesos.HealthCheck{GracePeriodSeconds: &gracePeriod}, healthResults, healthStates)

	err := errors.New("Error")

	healthResults <- err

	state := <-healthStates
	assert.Equal(t, Event{Type: Unhealthy, Message: err.Error()}, state)

	healthResults <- err

	state = <-healthStates
	assert.Equal(t, Event{Type: Unhealthy, Message: err.Error()}, state)

	assert.Empty(t, healthStates)
}

func TestHandleHealthResultsShouldSetKillToTrueWhenConsecutiveFailuresExceedsConfiguration(t *testing.T) {
	healthResults := make(chan error)
	healthStates := make(chan Event)
	gracePeriod := 0.0
	maxConsecutiveFailures := uint32(1)
	check := mesos.HealthCheck{
		GracePeriodSeconds:  &gracePeriod,
		ConsecutiveFailures: &maxConsecutiveFailures,
	}
	go handleHealthResults(check, healthResults, healthStates)

	err := errors.New("Error")

	healthResults <- err

	state := <-healthStates
	assert.Equal(t, Event{Type: FailedDueToUnhealthy, Message: err.Error()}, state)
	assert.Empty(t, healthStates)
}

func TestHandleHealthResultsShouldNotNotifyWhenTaskIsHealthy(t *testing.T) {
	healthResults := make(chan error)
	healthStates := make(chan Event)
	gracePeriod := 0.0
	maxConsequeltialFailures := uint32(2)
	check := mesos.HealthCheck{
		GracePeriodSeconds:  &gracePeriod,
		ConsecutiveFailures: &maxConsequeltialFailures,
	}
	go handleHealthResults(check, healthResults, healthStates)

	err := errors.New("Error")

	healthResults <- err
	state := <-healthStates
	assert.Equal(t, Event{Type: Unhealthy, Message: err.Error()}, state)

	healthResults <- nil
	state = <-healthStates
	assert.Equal(t, Event{Type: Healthy}, state)

	healthResults <- nil
	assert.Empty(t, healthStates, "Notification should NOT be sent for recurring healthy states")
}

func TestHandleHealthResultsShouldNotNotifyWhenTaskIsInGracePeriod(t *testing.T) {
	healthResults := make(chan error)
	healthStates := make(chan Event)
	gracePeriod := 200.0
	maxConsequeltialFailures := uint32(2)
	check := mesos.HealthCheck{
		GracePeriodSeconds:  &gracePeriod,
		ConsecutiveFailures: &maxConsequeltialFailures,
	}
	go handleHealthResults(check, healthResults, healthStates)

	err := errors.New("Error")

	healthResults <- err
	assert.Empty(t, healthStates, "Notification should NOT be sent for failing task during grace period")
}

func TestNewHealthCheckShouldReturnNilWhenCheckPasses(t *testing.T) {
	commandType := mesos.HealthCheck_COMMAND
	validCommand := "true"
	command := mesos.CommandInfo{Value: &validCommand}
	check := mesos.HealthCheck{Type: &commandType, Command: &command}

	healthCheck := newHealthCheck(check)
	err := healthCheck()
	assert.NoError(t, err)
}

func TestNewHealthCheckShouldReturnErrorOnUnknownCheck(t *testing.T) {
	healthCheck := newHealthCheck(mesos.HealthCheck{})
	err := healthCheck()
	assert.EqualError(t, err, "Unknown health check type: UNKNOWN")
}

func TestNewHealthCheckShouldReturnErrorOnUdefinedCommand(t *testing.T) {
	commandType := mesos.HealthCheck_COMMAND
	healthCheck := newHealthCheck(mesos.HealthCheck{Type: &commandType})
	err := healthCheck()
	assert.EqualError(t, err, "Unknown health check type: COMMAND")
}

func TestCommandHealthCheckShouldReturnErrorOnEmptyCheck(t *testing.T) {
	commandType := mesos.HealthCheck_COMMAND
	check := mesos.HealthCheck{Type: &commandType}

	err := commandHealthCheck(check)
	assert.EqualError(t, err, "Command health check not defined")
}

func TestCommandHealthCheckShouldReturnErrorOnInvalidCommand(t *testing.T) {
	invalidCommand := "false"
	command := mesos.CommandInfo{Value: &invalidCommand}
	check := mesos.HealthCheck{Command: &command}
	err := commandHealthCheck(check)
	assert.EqualError(t, err, "Command health check errored: exit status 1")
}

func TestCommandHealthCheckShouldReturnErrorOnInvalidCheck(t *testing.T) {
	cmd := "test $X"
	env := mesos.Environment{
		Variables: []mesos.Environment_Variable{{Name: "X", Value: "1"}},
	}
	command := mesos.CommandInfo{Value: &cmd, Environment: &env}
	check := mesos.HealthCheck{Command: &command}
	err := commandHealthCheck(check)
	assert.NoError(t, err)
}

func TestCommandHealthCheckShouldReturnErrorOnInvalidCommandForNonShell(t *testing.T) {
	commandType := mesos.HealthCheck_COMMAND

	invalidCommand := "(true)"
	shell := false
	command := mesos.CommandInfo{
		Value: &invalidCommand,
		Shell: &shell,
	}
	check := mesos.HealthCheck{Type: &commandType, Command: &command}

	err := commandHealthCheck(check)
	assert.EqualError(t, err, "Command health check errored: exec: \"(true)\": executable file not found in $PATH")
}

func TestCommandHealthCheckShouldReturnErrorOnAfterTimeout(t *testing.T) {
	command := mesos.CommandInfo{}
	sleep := "sleep 1"
	command.Value = &sleep
	timeout := 0.0
	check := mesos.HealthCheck{Command: &command, TimeoutSeconds: &timeout}
	err := commandHealthCheck(check)
	assert.EqualError(t, err, "Command health check timed out after 0s")
}

func TestIfTCPHealthCheckPassesWhenPortIsOpen(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	check := buildTCPCheckForTestServer(ts, 0.1)

	err := tcpHealthCheck(check)
	assert.NoError(t, err)
}

func TestIfTCPHealthCheckFailWhenPortIsClosed(t *testing.T) {
	check := buildTCPCheck(0, 0.1)

	err := tcpHealthCheck(check)
	assert.Error(t, err)
}

func TestIfHTTPHealthCheckWithoutPathPassesWhenOKStatusCodeIsReceived(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	check := buildHTTPCheckForTestServer(ts, 0.1, "")

	err := httpHealthCheck(check)

	assert.NoError(t, err)
}

func TestIfHTTPHealthCheckWithPathPassesWhenOKStatusCodeIsReceived(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.URL.Path)
		if r.URL.Path == "/status/info" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()
	check := buildHTTPCheckForTestServer(ts, 0.1, "/status/info")

	err := httpHealthCheck(check)

	assert.NoError(t, err)
}

func TestIfHTTPHealthCheckFailsWhenTimeoutOccur(t *testing.T) {
	sleep := make(chan bool)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-sleep // wait until timeout occurs
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	check := buildHTTPCheckForTestServer(ts, time.Millisecond.Seconds(), "")

	err := httpHealthCheck(check)
	close(sleep) // release the server

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Client.Timeout exceeded while awaiting headers")
}

func TestIfHTTPHealthCheckFailsWhenRequestIsInvalid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()
	check := buildHTTPCheckForTestServer(ts, 0.1, "")

	err := httpHealthCheck(check)

	assert.EqualError(t, err, "Health check error: received status code 400, but expected codes between 200 and 399")
}

func TestIfHTTPHealthCheckFailsWhenServiceIsUnavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	check := buildHTTPCheckForTestServer(ts, 0.1, "")

	err := httpHealthCheck(check)

	assert.EqualError(t, err, "Health check error: received status code 503, but expected codes between 200 and 399")
}

func TestIfHTTPHealthCheckFailsWhenNoServiceIsListeningOnConfiguredPort(t *testing.T) {
	check := buildHTTPCheck("http", 1000, "/", 0.1)

	err := httpHealthCheck(check)

	assert.Error(t, err)
}

func buildHTTPCheck(scheme string, port uint32, path string, timeoutSeconds float64) mesos.HealthCheck {
	return mesos.HealthCheck{
		HTTP: &mesos.HealthCheck_HTTPCheckInfo{
			Path:   &path,
			Port:   port,
			Scheme: &scheme,
		},
		TimeoutSeconds: &timeoutSeconds,
	}
}

func buildHTTPCheckForTestServer(ts *httptest.Server, timeoutSeconds float64, path string) mesos.HealthCheck {
	testServerURL, _ := url.Parse(ts.URL)
	testServerPort, _ := strconv.Atoi(testServerURL.Port())
	return buildHTTPCheck(testServerURL.Scheme, uint32(testServerPort), path, timeoutSeconds)
}

func buildTCPCheckForTestServer(ts *httptest.Server, timeoutSeconds float64) mesos.HealthCheck {
	testServerURL, _ := url.Parse(ts.URL)
	testServerPort, _ := strconv.Atoi(testServerURL.Port())
	return buildTCPCheck(uint32(testServerPort), timeoutSeconds)
}

func buildTCPCheck(port uint32, timeoutSeconds float64) mesos.HealthCheck {
	return mesos.HealthCheck{
		TCP: &mesos.HealthCheck_TCPCheckInfo{
			Port: port,
		},
		TimeoutSeconds: &timeoutSeconds,
	}
}
