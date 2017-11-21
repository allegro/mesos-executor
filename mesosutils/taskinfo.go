package mesosutils

import (
	"fmt"
	"strconv"
	"strings"

	mesos "github.com/mesos/mesos-go/api/v1/lib"
)

// serviceIDLabelKey is the name of a Mesos label used for defining a custom service name
const serviceIDLabelKey = "serviceId"

// TaskInfo is a wrapper over Mesos TaskInfo
// providing convenient ways to access some of its values
type TaskInfo struct {
	TaskInfo mesos.TaskInfo
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
	return 0, fmt.Errorf("No weight defined")
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
	keys := []string{}

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
