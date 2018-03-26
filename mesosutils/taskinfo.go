package mesosutils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	mesos "github.com/mesos/mesos-go/api/v1/lib"
)

// serviceIDLabelKey is the name of a Mesos label used for defining a custom service name
const serviceIDLabelKey = "serviceId"

// TaskInfo is a wrapper over Mesos TaskInfo
// providing convenient ways to access some of its values
type TaskInfo struct {
	TaskInfo mesos.TaskInfo
}

// HealthCheckType is type of healthcheck
type HealthCheckType int

const (
	// NONE represents empty healthcheck
	NONE HealthCheckType = iota
	// HTTP indicates HTTP field in HealthCheck is configured
	HTTP
	// TCP indicates healthcheck is TCP
	TCP
	// COMMAND indicates healthcheck is a command
	COMMAND
)

// HealthCheck keeps details how to check task health
type HealthCheck struct {
	// Type specify healthcheck type
	Type HealthCheckType
	// Interval specify how often healthcheck should be performed
	Interval time.Duration
	// Timeout specify duration after healtcheck should be aborted and marked as unhealthy
	Timeout time.Duration
	// HTTP contains details about heatlhcheck when HTTP Type is set
	HTTP HTTPCheck
	//TODO(janisz): Implement other healthchecks
}

// HTTPCheck contains details about HTTP healthcheck
type HTTPCheck struct {
	Path string
}

// TaskID is framework-generated ID to distinguish a task
type TaskID string

// GetTaskID returns id of the task
func (h TaskInfo) GetTaskID() TaskID {
	return TaskID(h.TaskInfo.GetTaskID().Value)
}

// GetHealthCheck returns
func (h TaskInfo) GetHealthCheck() (check HealthCheck) {
	mesosCheck := h.TaskInfo.GetHealthCheck()

	if mesosCheck == nil ||
		mesosCheck.IntervalSeconds == nil ||
		mesosCheck.TimeoutSeconds == nil {
		check.Type = NONE
		return check
	}

	check.Interval = Duration(*mesosCheck.IntervalSeconds)
	check.Timeout = Duration(*mesosCheck.TimeoutSeconds)

	if mesosCheck.HTTP != nil {
		check.Type = HTTP
		check.HTTP = HTTPCheck{Path: mesosCheck.GetHTTP().GetPath()}
	}
	if mesosCheck.TCP != nil {
		check.Type = TCP
	}

	return check
}

// FindLabel returns a label matching given key
func (h TaskInfo) FindLabel(key string) *mesos.Label {
	return FindLabel(h.TaskInfo.GetLabels().GetLabels(), key)
}

// GetLabelKeysByValue returns all keys of labels that contain a given value
func (h TaskInfo) GetLabelKeysByValue(value string) []string {
	return GetLabelKeysByValue(h.TaskInfo.GetLabels().GetLabels(), value)
}

// GetLabelValue returns value for of a label
func (h TaskInfo) GetLabelValue(key string) string {
	if label := h.FindLabel(key); label != nil {
		return label.GetValue()
	}
	return ""
}

// GetWeight return a initial weight of the task.
// If weight is not set or has malformed format then returned
// weight is 0 and error is not nil.
func (h TaskInfo) GetWeight() (int, error) {
	for _, tags := range h.GetLabelKeysByValue("tag") {
		const weightPrefix = "weight:"
		if strings.HasPrefix(tags, weightPrefix) {
			return strconv.Atoi(strings.TrimPrefix(tags, weightPrefix))
		}
	}
	return 0, fmt.Errorf("no weight defined")
}

// GetPorts returns a list of task ports
func (h TaskInfo) GetPorts() []mesos.Port {
	return h.TaskInfo.GetDiscovery().GetPorts().GetPorts()
}

// FindEnvValue returns the value of an environment variable
func (h TaskInfo) FindEnvValue(key string) string {
	for _, envVar := range h.TaskInfo.GetCommand().GetEnvironment().GetVariables() {
		if envVar.GetName() == key {
			return envVar.GetValue()
		}
	}
	return ""
}

// GetServiceID extracts service ID from labels in mesos TaskInfo. If it fails
// to do so, it returns executor ID.
func (h TaskInfo) GetServiceID() string {
	serviceID := h.GetLabelValue(serviceIDLabelKey)
	if serviceID != "" {
		return serviceID
	}

	executorID := h.TaskInfo.GetExecutor().GetExecutorID()

	return executorID.GetValue()
}

// GetLabelKeysByValue searches provided labels for a given value and returns a list of matching keys
func GetLabelKeysByValue(labels []mesos.Label, value string) []string {
	var keys []string

	for _, label := range labels {
		if label.GetValue() == value {
			keys = append(keys, label.GetKey())
		}
	}

	return keys
}

// FindLabel returns a label matching given key
func FindLabel(labels []mesos.Label, key string) *mesos.Label {
	for _, label := range labels {
		if label.GetKey() == key {
			return &label
		}
	}
	return nil
}
