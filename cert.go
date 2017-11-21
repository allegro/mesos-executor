package executor

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// GetCertFromEnvVariables returns certificate stored in
// environment variables. If no certificate is found then
// empty string is returned
func GetCertFromEnvVariables(env []string) (*x509.Certificate, error) {
	for _, value := range env {
		if strings.HasPrefix(value, "CERTIFICATE=") {
			pemEncoded := []byte(strings.TrimPrefix(value, "CERTIFICATE="))
			p, _ := pem.Decode(pemEncoded)

			if p == nil {
				return nil, errors.New("Missing certificate data")
			}

			cert, err := x509.ParseCertificate(p.Bytes)
			if err != nil {
				return nil, fmt.Errorf("Certificate is invalid: %s", err)
			}
			return cert, nil
		}
	}
	return nil, errors.New("Missing certificate")
}
