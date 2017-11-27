package metrics

import (
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
