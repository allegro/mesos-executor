package executor

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIfReturnsErrorWhenNoValidCertificateFound(t *testing.T) {
	testCases := []struct {
		err string
		env []string
	}{
		{"Missing certificate", nil},
		{"Missing certificate", []string{
			"ALLEGRO_PKI_SENSITIVE_VAR=xxx",
			"NOT_SENSITIVE=xxx",
		}},
		{"Missing certificate data", []string{
			"ALLEGRO_PKI_SENSITIVE_VAR=xxx",
			"CERTIFICATE=xxx",
			"NOT_SENSITIVE=xxx",
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.err, func(t *testing.T) {
			cert, err := GetCertFromEnvVariables(tc.env)
			assert.Nil(t, cert)
			assert.EqualError(t, err, tc.err)
		})
	}
}

func TestIfReturnscertificateFromArgs(t *testing.T) {
	pemCert, err := ioutil.ReadFile("testdata/cert.pem")
	require.NoError(t, err)

	cert, err := GetCertFromEnvVariables([]string{
		"ALLEGRO_PKI_SENSITIVE_VAR=xxx",
		"CERTIFICATE=" + string(pemCert),
		"CERTIFICATE=yy",
		"NOT_SENSITIVE=xxx",
	})
	assert.NoError(t, err)
	assert.Equal(t, "Vault CA5", cert.Issuer.CommonName)
	assert.Equal(t, "2017-06-13 13:53:05 +0000 UTC", cert.NotAfter.String())
}
