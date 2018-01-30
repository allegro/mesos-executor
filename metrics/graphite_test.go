package metrics

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIfFailsToSetupGraphiteWithInvalidConfig(t *testing.T) {
	cfg := GraphiteConfig{
		Host: "!@#$",
	}
	err := SetupGraphite(cfg)

	assert.Error(t, err)
}

func TestIfNotFailsToSetupGraphiteWithValidConfig(t *testing.T) {
	cfg := GraphiteConfig{
		Host: "localhost",
		Port: 2003,
	}
	err := SetupGraphite(cfg)

	assert.NoError(t, err)
}

func TestIfBuildsCorrectMetricsPrefix(t *testing.T) {
	metricsID = "uuid"
	testCases := []struct {
		hostname       string
		expectedPrefix string
	}{
		{"localhost", "basePrefix.localhost.uuid"},
		{"my.host.with.dots", "basePrefix.my_host_with_dots.uuid"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("hostname=%s", tc.hostname), func(t *testing.T) {
			os.Setenv("MESOS_HOSTNAME", tc.hostname)
			defer os.Unsetenv("MESOS_HOSTNAME")

			actualPrefix := buildUniquePrefix("basePrefix")
			assert.Equal(t, tc.expectedPrefix, actualPrefix)
		})
	}
}
