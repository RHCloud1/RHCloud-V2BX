package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegoRenewMalformedCertificate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(p, []byte("invalid certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	l := &Lego{config: certConfig("example.com", p, p+".key", "http")}
	if err := l.RenewCert(); err == nil {
		t.Fatal("malformed certificates must not be silently ignored")
	}
}
