package node

import (
	"path/filepath"
	"testing"
	"time"

	certcheck "github.com/InazumaV/V2bX/common/cert"
	"github.com/InazumaV/V2bX/conf"
)

func certConfig(domain, certFile, keyFile, mode string) *conf.CertConfig {
	return &conf.CertConfig{CertDomain: domain, CertFile: certFile, KeyFile: keyFile, CertMode: mode}
}

func TestSelfCertificate(t *testing.T) {
	for _, domain := range []string{"example.com", "127.0.0.1", "::1"} {
		t.Run(domain, func(t *testing.T) {
			dir := t.TempDir()
			certFile, keyFile := filepath.Join(dir, "tls", "fullchain.cer"), filepath.Join(dir, "tls", "cert.key")
			if err := generateSelfSslCertificate(domain, certFile, keyFile); err != nil {
				t.Fatal(err)
			}
			if err := certcheck.Check(certFile, keyFile, domain, time.Now()); err != nil {
				t.Fatal(err)
			}
			if err := certcheck.Check(certFile, keyFile, "wrong.example.com", time.Now()); err == nil {
				t.Fatal("expected a SAN mismatch")
			}
			if err := certcheck.Check(certFile, keyFile, domain, time.Now().AddDate(31, 0, 0)); err == nil {
				t.Fatal("expected an expired certificate error")
			}
		})
	}
}

func TestRequestCertRejectsWrongDomain(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "fullchain.cer"), filepath.Join(dir, "cert.key")
	if err := generateSelfSslCertificate("hy.example.com", certFile, keyFile); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"file", "http", "dns", "self"} {
		c := &Controller{Options: &conf.Options{CertConfig: certConfig("any.example.com", certFile, keyFile, mode)}}
		if err := c.requestCert(); err == nil {
			t.Fatalf("mode %s reused another domain's certificate", mode)
		}
	}
}
