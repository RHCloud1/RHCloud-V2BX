package cert

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"
)

// Check validates the key pair and leaf identity, without requiring a public CA.
func Check(certFile, keyFile, domain string, now time.Time) error {
	leaf, err := Identity(certFile, keyFile, domain)
	if err != nil {
		return err
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return fmt.Errorf("certificate validity is %s to %s (current time %s)",
			leaf.NotBefore.UTC().Format(time.RFC3339), leaf.NotAfter.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	}
	return nil
}

// Identity permits expired managed certificates to reach the renewal path.
func Identity(certFile, keyFile, domain string) (*x509.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate/key: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse TLS certificate: %w", err)
	}
	if domain != "" {
		if err := leaf.VerifyHostname(domain); err != nil {
			return nil, fmt.Errorf("certificate does not cover CertDomain %q: %w", domain, err)
		}
	}
	return leaf, nil
}
