package runenv

import (
	"fmt"
	"net"
	"os"
	"regexp"
)

const (
	// LocalEnv represents local environment.
	LocalEnv = Env("local")
	// DevEnv represents development environment.
	DevEnv = Env("dev")
	// TestEnv represents test environment.
	TestEnv = Env("test")
	// ProdEnv represents production environment.
	ProdEnv = Env("prod")

	defaultEnvironment = LocalEnv
)

var environmentRegexp = regexp.MustCompile(`.*-(prod|test|dev)\..*`)

var getOsHostname = OsHostname

// Env is name of the environment on which service is running.
type Env string

// Environment returns current environment based on hostname. If it cannot determine
// the hostname it returns an error.
func Environment() (Env, error) {
	hostname, err := Hostname()

	if err != nil {
		return defaultEnvironment, err
	}

	matches := environmentRegexp.FindStringSubmatch(hostname)

	if matches == nil || len(matches) == 1 {
		return defaultEnvironment, nil
	}

	return Env(matches[1]), nil
}

// AvailabilityZone return the name of runtime availability zone. It returns
// empty string with erro if it cannot determine the name.
func AvailabilityZone() (string, error) {
	return getEnvVarIfSet("CLOUD_AVAILABILITY_ZONE")
}

// Datacenter returns the name of runtime datacenter. It returns empty string with
// error if it cannot determine the name.
func Datacenter() (string, error) {
	return getEnvVarIfSet("CLOUD_DC")
}

// Hostname returns the host name reported by Mesos, cloud or operating system.
func Hostname() (string, error) {
	if os.Getenv("MESOS_HOSTNAME") != "" {
		return os.Getenv("MESOS_HOSTNAME"), nil
	}
	if os.Getenv("CLOUD_HOSTNAME") != "" {
		return os.Getenv("CLOUD_HOSTNAME"), nil
	}
	return getOsHostname()
}

// IP returns the IP of runtime host.
func IP() net.IP {
	return net.ParseIP(os.Getenv("CLOUD_PUBLIC_IP"))
}

// MarathonAppID returns ID of Marathon application in which context the process
// is running. It returns empty string with error if it cannot determine the ID.
func MarathonAppID() (string, error) {
	return getEnvVarIfSet("MARATHON_APP_ID")
}

// Region returns the name of runtime cloud region. It returns empty string with
// error if it cannot determine the name.
func Region() (string, error) {
	return getEnvVarIfSet("CLOUD_REGION")
}

// TaskID returns mesos task ID. It returns empty string with error if it cannot
// determine the ID.
func TaskID() (string, error) {
	return getEnvVarIfSet("MESOS_TASK_ID")
}

// ExecutorID returns mesos executor ID. It returns empty string with error if it cannot
// determine the ID.
func ExecutorID() (string, error) {
	return getEnvVarIfSet("MESOS_EXECUTOR_ID")
}

// MesosAgentEndpoint returns Mesos Agent endpoint. It returns empty string with error
// if it cannot determine Agent endpoint.
func MesosAgentEndpoint() (string, error) {
	return getEnvVarIfSet("MESOS_AGENT_ENDPOINT")
}

// getEnvVarIfSet returns environment variable value when it is set
// or error when it's empty or not set
func getEnvVarIfSet(name string) (string, error) {
	if os.Getenv(name) != "" {
		return os.Getenv(name), nil
	}
	return "", fmt.Errorf("no %s environment variable set", name)
}
