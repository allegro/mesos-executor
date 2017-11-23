package runenv

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var environmentTests = []struct {
	mesosHostname string
	cloudHostname string
	environment   Env
}{
	{"-dev.", "-dev.", DevEnv},
	{"-test.", "", TestEnv},
	{"-prod.", "", ProdEnv},
	{"-unknown.", "", LocalEnv},
	{"", "-dev.", DevEnv},
	{"", "-prod.", ProdEnv},
}

func TestEnvironmentParsingFromEnvVariables(t *testing.T) {
	for _, environmentTest := range environmentTests {
		os.Clearenv()

		if environmentTest.mesosHostname != "" {
			_ = os.Setenv("MESOS_HOSTNAME", environmentTest.mesosHostname)
		}
		if environmentTest.cloudHostname != "" {
			_ = os.Setenv("CLOUD_HOSTNAME", environmentTest.cloudHostname)
		}
		env, err := Environment()

		require.NoError(t, err)
		assert.Equal(t, Env(environmentTest.environment), env)
	}
}

func TestGetEnvVarIfSetGetsEnvVar(t *testing.T) {
	_ = os.Setenv("TEST_ENV1", "TEST_VALUE1")

	value, err := getEnvVarIfSet("TEST_ENV1")

	assert.NoError(t, err)
	assert.Equal(t, "TEST_VALUE1", value)
}

func TestGetEnvVarIfSetReturnsErrorForEmptyValue(t *testing.T) {
	_ = os.Setenv("TEST_ENV2", "")

	value, err := getEnvVarIfSet("TEST_ENV2")

	assert.Error(t, err)
	assert.Empty(t, value)
}

func TestGetEnvVarIfSetReturnErrorIfVarDoesntExist(t *testing.T) {
	_ = os.Unsetenv("TEST_ENV3")

	value, err := getEnvVarIfSet("TEST_ENV3")

	assert.Error(t, err)
	assert.Empty(t, value)
}

func TestGetEnvWithNoVariablesReturnsOsFqdnBasedEnv(t *testing.T) {
	os.Clearenv()

	getOsHostname = func() (string, error) { return "expected.hostname-prod.fqdn", nil }

	env, err := Environment()

	assert.NoError(t, err)
	assert.Equal(t, ProdEnv, env)
}
