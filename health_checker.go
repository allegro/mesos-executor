package executor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	mesos "github.com/mesos/mesos-go/api/v1/lib"
	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor/mesosutils"
	"github.com/allegro/mesos-executor/runenv"
)

// Default for the http health check <host> part.
// See: https://github.com/apache/mesos/blob/1.1.3/include/mesos/mesos.proto#L353-L357
const defaultDomain = "127.0.0.1"

// DoHealthChecks schedules health check defined in check.
// HealthState updates are delivered on provided healthStates channel.
func DoHealthChecks(check mesos.HealthCheck, healthStates chan<- Event) {
	log.Debugf("Health check configuration: %s", check.String())
	performCheck := newHealthCheck(check)
	delay := mesosutils.Duration(check.GetDelaySeconds())

	healthResults := make(chan error)
	go handleHealthResults(check, healthResults, healthStates)

	interval := mesosutils.Duration(check.GetIntervalSeconds())

	log.Infof("Scheduling health check for task in %s", delay)
	time.AfterFunc(delay, func() {

		healthResults <- performCheck()

		log.Infof("Scheduling health check for task every %s", interval)
		tick := time.NewTicker(interval)
		for range tick.C {
			healthResults <- performCheck()
		}
	})
}

func handleHealthResults(checkDefinition mesos.HealthCheck, healthResults <-chan error, healthStates chan<- Event) {
	neverPassedBefore := true
	delay := mesosutils.Duration(checkDefinition.GetDelaySeconds())
	startTime := time.Now().Truncate(delay)
	var consecutiveFailures uint32

	for err := range healthResults {
		if err != nil {
			if neverPassedBefore && time.Since(startTime).Seconds() < checkDefinition.GetGracePeriodSeconds() {
				log.WithError(err).Info("Ignoring failure of health check: still in grace period")
				continue
			}
			consecutiveFailures++
			log.WithError(err).Infof("Health check for task failed %d times consecutively", consecutiveFailures)

			// Even if we send the `FailedDueToUnhealthy` type, it is an executor who kills the task
			// and honors the type (or not). We have no control over the task's lifetime,
			// hence we should continue until we are explicitly asked to stop.
			if consecutiveFailures >= checkDefinition.GetConsecutiveFailures() {
				healthStates <- Event{Type: FailedDueToUnhealthy, Message: err.Error()}
			} else {
				healthStates <- Event{Type: Unhealthy, Message: err.Error()}
			}
			continue
		}
		// Send a healthy status update on the first success,
		// and on the first success following failure(s).
		if neverPassedBefore || consecutiveFailures > 0 {
			log.Info("Health check passed")
			healthStates <- Event{Type: Healthy}
		}
		consecutiveFailures = 0
		neverPassedBefore = false
	}
}

// healthCheck is a function that performs health check. It returns an error if
// health check failed or nil if health check passed.
type healthCheckFunction func() error

// NewHealthCheck returns health check that performs check given as a configuration.
func newHealthCheck(check mesos.HealthCheck) healthCheckFunction {
	// For backward compatibility with Mesos 1.0.0 we can't rely on GetType() here.
	// See: https://lists.apache.org/thread.html/ec6139491c36a4387ffad4b1e29e3bbce16d99ad0620e1d72e26bc58@%3Cuser.mesos.apache.org%3E
	if check.GetCommand() != nil {
		return func() error { return commandHealthCheck(check) }
	} else if check.GetHTTP() != nil {
		return func() error { return httpHealthCheck(check) }
	} else if check.GetTCP() != nil {
		return func() error { return tcpHealthCheck(check) }
	}

	return func() error { return fmt.Errorf("Unknown health check type: %s", check.GetType()) }
}

func commandHealthCheck(checkDefinition mesos.HealthCheck) error {
	if checkDefinition.GetCommand() == nil {
		return errors.New("Command health check not defined")
	}

	timeout := mesosutils.Duration(checkDefinition.GetTimeoutSeconds())
	ctx, _ := context.WithTimeout(context.Background(), timeout) // nolint: vet

	commandInfo := checkDefinition.GetCommand()
	var cmd *exec.Cmd
	// From: https://github.com/apache/mesos/blob/1.1.0/include/mesos/mesos.proto#L509-L521
	// There are two ways to specify the command:
	if commandInfo.GetShell() {
		// the command will be launched via shell
		// (i.e., /bin/sh -c 'value'). The 'value' specified will be
		// treated as the shell command. The 'arguments' will be ignored.
		cmd = exec.CommandContext(ctx, "sh", "-c", commandInfo.GetValue()) // #nosec
	} else {
		// the command will be launched by passing
		// arguments to an executable. The 'value' specified will be
		// treated as the filename of the executable. The 'arguments'
		// will be treated as the arguments to the executable. This is
		// similar to how POSIX exec families launch processes (i.e.,
		// execlp(value, arguments(0), arguments(1), ...)).
		cmd = exec.CommandContext(ctx, commandInfo.GetValue(), commandInfo.GetArguments()...) // #nosec
	}
	// Copy system environment
	environ := os.Environ()
	// Append check custom environment
	for _, variable := range commandInfo.GetEnvironment().GetVariables() {
		environ = append(environ, fmt.Sprintf("%s=%s", variable.Name, variable.Value))
	}
	cmd.Env = environ
	// Redirect command output
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	log.Infof("Launching command health check: %s", strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.WithError(ctx.Err()).Info("Command health check timed out")
			return fmt.Errorf("Command health check timed out after %s", timeout)
		}
		return fmt.Errorf("Command health check errored: %s", err)
	}
	return nil
}

func tcpHealthCheck(checkDefinition mesos.HealthCheck) error {
	timeout := mesosutils.Duration(checkDefinition.GetTimeoutSeconds())
	address := HealthCheckAddress(checkDefinition.GetTCP().GetPort())
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Errorf("TCP health error: %s", err)
	}
	if err := conn.Close(); err != nil {
		log.WithError(err).Warn("Error closing TCP health check connection")
	}
	return nil
}

func httpHealthCheck(checkDefinition mesos.HealthCheck) error {
	const defaultHTTPScheme = "http"

	timeout := mesosutils.Duration(checkDefinition.GetTimeoutSeconds())
	client := &http.Client{
		Timeout: timeout,
	}

	var checkURL url.URL
	checkURL.Host = HealthCheckAddress(checkDefinition.GetHTTP().GetPort())
	checkURL.Path = checkDefinition.GetHTTP().GetPath()
	if checkDefinition.GetHTTP().Scheme != nil {
		checkURL.Scheme = checkDefinition.GetHTTP().GetScheme()
	} else {
		checkURL.Scheme = defaultHTTPScheme
	}

	response, err := client.Get(checkURL.String())
	if err != nil {
		return fmt.Errorf("Health check error: %s", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.WithError(err).Warn("Error closing HTTP health check response body")
		}
	}()

	// Default executors treat return codes between 200 and 399 as success
	// See: https://github.com/apache/mesos/blob/1.1.3/include/mesos/mesos.proto#L355-L357
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return fmt.Errorf("Health check error: received status code %d, but expected codes between 200 and 399", response.StatusCode)
	}

	return nil
}

// HealthCheckAddress returns host and port that should be used for health checking
// service.
func HealthCheckAddress(port uint32) string {
	ip := runenv.IP()
	var host string
	if ip == nil {
		host = defaultDomain
	} else {
		host = ip.String()
	}
	return fmt.Sprintf("%s:%d", host, port)
}
