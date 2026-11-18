package node

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"

	"github.com/InazumaV/V2bX/common/file"
)

func TestGuardACMECoolsDownAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	config := certConfig("hy.example.com", filepath.Join(dir, "fullchain.cer"), filepath.Join(dir, "cert.key"), "dns")
	calls := 0
	failing := func() error { calls++; return errors.New("acme: error: 400 :: urn:ietf:params:acme:error:connection") }
	if err := guardACME(config, failing); err == nil || calls != 1 {
		t.Fatalf("first request: err=%v calls=%d", err, calls)
	}
	// A restarted process reads the same file and must not contact the CA again.
	for i := 0; i < 3; i++ {
		err := guardACME(config, failing)
		if err == nil || !strings.Contains(err.Error(), "acme-cooldown.json") || !strings.Contains(err.Error(), "connection") {
			t.Fatalf("cooldown error: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("CA contacted %d times during cooldown", calls)
	}

	expired, _ := json.Marshal(acmeCooldown{Failures: 1, Until: time.Now().Add(-time.Second)})
	if err := os.WriteFile(acmeCooldownPath(config.CertFile), expired, 0644); err != nil {
		t.Fatal(err)
	}
	if err := guardACME(config, func() error { calls++; return nil }); err != nil || calls != 2 {
		t.Fatalf("request after cooldown: err=%v calls=%d", err, calls)
	}
	if file.IsExist(acmeCooldownPath(config.CertFile)) {
		t.Fatal("success should clear the cooldown")
	}
}

func TestRecordACMEFailureBackoff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acme-cooldown.json")
	now := time.Date(2026, 11, 18, 2, 0, 0, 0, time.UTC)
	for i, want := range []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 40 * time.Minute, time.Hour, time.Hour} {
		if s := recordACMEFailure(path, errors.New("failed"), now); s.Until.Sub(now) != want || s.Failures != i+1 {
			t.Fatalf("failure %d: wait %s, want %s", i+1, s.Until.Sub(now), want)
		}
	}
}

func TestRecordACMEFailureHonorsRetryAfter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acme-cooldown.json")
	now := time.Date(2026, 11, 18, 2, 13, 10, 0, time.UTC)
	limited := errors.New(`acme: error: 429 :: POST :: https://acme-v02.api.letsencrypt.org/acme/new-order :: urn:ietf:params:acme:error:rateLimited :: too many failed authorizations (5) for "host.ffhy.uk" in the last 1h0m0s, retry after 2026-11-18 03:00:05 UTC: see https://letsencrypt.org/docs/rate-limits/`)
	if s := recordACMEFailure(path, limited, now); !s.Until.Equal(time.Date(2026, 11, 18, 3, 0, 5, 0, time.UTC)) {
		t.Fatalf("retry-after ignored: %s", s.Until)
	}
}

func TestGuardACMEBusyPort80SkipsCA(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	old := httpChallengePort
	_, httpChallengePort, _ = net.SplitHostPort(ln.Addr().String())
	defer func() { httpChallengePort = old }()

	dir := t.TempDir()
	config := certConfig("hy.example.com", filepath.Join(dir, "fullchain.cer"), filepath.Join(dir, "cert.key"), "http")
	err = guardACME(config, func() error { t.Fatal("CA contacted while the challenge port is busy"); return nil })
	if err == nil || !strings.Contains(err.Error(), "未向 Let's Encrypt 发起申请") || !strings.Contains(err.Error(), "dns 模式") {
		t.Fatalf("busy port error: %v", err)
	}
	if file.IsExist(acmeCooldownPath(config.CertFile)) {
		t.Fatal("a local port check must not start a cooldown")
	}
}
