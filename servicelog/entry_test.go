package servicelog

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIfAddsSystemDataToLogEntry(t *testing.T) {
	os.Setenv("MESOS_HOSTNAME", "hostname")
	defer os.Unsetenv("MESOS_HOSTNAME")

	extender := SystemDataExtender{}
	logEntry := Entry{"key": "value"}

	extendedLogEntry := extender.Extend(logEntry)

	assert.Len(t, logEntry, 1)
	assert.Len(t, extendedLogEntry, 2)
	assert.Equal(t, "value", extendedLogEntry["key"])
	assert.Equal(t, "hostname", extendedLogEntry["srchost"])
}

func TestIfAddsStaticDataToLogEntry(t *testing.T) {
	extender := StaticDataExtender{
		Data: map[string]interface{}{
			"data1": "data1",
			"data2": "data2",
		},
	}
	logEntry := Entry{"key": "value"}

	extendedLogEntry := extender.Extend(logEntry)

	assert.Len(t, logEntry, 1)
	assert.Len(t, extendedLogEntry, 3)
	assert.Equal(t, "data1", extendedLogEntry["data1"])
	assert.Equal(t, "data2", extendedLogEntry["data2"])
}
