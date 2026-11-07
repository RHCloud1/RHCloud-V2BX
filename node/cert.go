package node

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	certcheck "github.com/InazumaV/V2bX/common/cert"
	"github.com/InazumaV/V2bX/common/file"
	log "github.com/sirupsen/logrus"
)

// ACME HTTP challenges share port 80; DNS providers use process-wide environment variables.
var certManagementMu sync.Mutex

func (c *Controller) renewCertTask() error {
	certManagementMu.Lock()
	defer certManagementMu.Unlock()
	l, err := NewLego(c.CertConfig)
	if err != nil {
		log.WithField("tag", c.tag).Error("new lego error: ", err)
		return nil
	}
	err = l.RenewCert()
	if err != nil {
		log.WithField("tag", c.tag).Error("renew cert error: ", err)
		return nil
	}
	return nil
}

func (c *Controller) requestCert() error {
	certManagementMu.Lock()
	defer certManagementMu.Unlock()
	if c.CertConfig == nil {
		return fmt.Errorf("CertConfig is required for TLS nodes")
	}
	switch c.CertConfig.CertMode {
	case "none", "":
	case "file":
		if c.CertConfig.CertFile == "" || c.CertConfig.KeyFile == "" {
			return fmt.Errorf("cert file path or key file path not exist")
		}
		return certcheck.Check(c.CertConfig.CertFile, c.CertConfig.KeyFile, c.CertConfig.CertDomain, time.Now())
	case "dns", "http":
		if c.CertConfig.CertFile == "" || c.CertConfig.KeyFile == "" {
			return fmt.Errorf("cert file path or key file path not exist")
		}
		if file.IsExist(c.CertConfig.CertFile) && file.IsExist(c.CertConfig.KeyFile) {
			leaf, err := certcheck.Identity(c.CertConfig.CertFile, c.CertConfig.KeyFile, c.CertConfig.CertDomain)
			if err != nil {
				return fmt.Errorf("existing certificate is invalid; use separate paths for different domains or replace this certificate: %w", err)
			}
			if !time.Now().Before(leaf.NotAfter) {
				l, err := NewLego(c.CertConfig)
				if err != nil {
					return err
				}
				if err := l.RenewCert(); err != nil {
					return fmt.Errorf("renew expired certificate: %w", err)
				}
			}
			return certcheck.Check(c.CertConfig.CertFile, c.CertConfig.KeyFile, c.CertConfig.CertDomain, time.Now())
		}
		l, err := NewLego(c.CertConfig)
		if err != nil {
			return fmt.Errorf("create lego object error: %s", err)
		}
		err = l.CreateCert()
		if err != nil {
			return fmt.Errorf("create lego cert error: %s", err)
		}
	case "self":
		if c.CertConfig.CertFile == "" || c.CertConfig.KeyFile == "" {
			return fmt.Errorf("cert file path or key file path not exist")
		}
		if file.IsExist(c.CertConfig.CertFile) && file.IsExist(c.CertConfig.KeyFile) {
			return certcheck.Check(c.CertConfig.CertFile, c.CertConfig.KeyFile, c.CertConfig.CertDomain, time.Now())
		}
		err := generateSelfSslCertificate(
			c.CertConfig.CertDomain,
			c.CertConfig.CertFile,
			c.CertConfig.KeyFile)
		if err != nil {
			return fmt.Errorf("generate self cert error: %s", err)
		}
	default:
		return fmt.Errorf("unsupported certmode: %s", c.CertConfig.CertMode)
	}
	return nil
}

func generateSelfSslCertificate(domain, certPath, keyPath string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		Version:      3,
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			CommonName: domain,
		},
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().AddDate(30, 0, 0),
	}
	if ip := net.ParseIP(domain); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{domain}
	}
	cert, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return err
	}
	for _, p := range []string{certPath, keyPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}), 0600); err != nil {
		return err
	}
	return os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	}), 0644)
}
