package node

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/goccy/go-json"

	"github.com/InazumaV/V2bX/common/file"
	"github.com/InazumaV/V2bX/conf"
)

// systemd restarts a failed V2bX every 10s. Without a persistent cooldown each
// restart sends a new ACME order, and Let's Encrypt soon answers 429
// (5 failed authorizations per domain per hour) for the rest of the hour.
const (
	acmeCooldownBase = 5 * time.Minute
	acmeCooldownMax  = time.Hour
)

// httpChallengePort is the TCP port lego's http-01 server binds; tests override it.
var httpChallengePort = "80"

var acmeRetryAfter = regexp.MustCompile(`retry after (\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC`)

type acmeCooldown struct {
	Failures int       `json:"failures"`
	Until    time.Time `json:"until"`
	Error    string    `json:"error"`
}

func acmeCooldownPath(certFile string) string {
	return filepath.Join(filepath.Dir(certFile), "acme-cooldown.json")
}

func loadACMECooldown(path string) acmeCooldown {
	var s acmeCooldown
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

func recordACMEFailure(path string, cause error, now time.Time) acmeCooldown {
	s := loadACMECooldown(path)
	s.Failures++
	wait := acmeCooldownMax
	if s.Failures <= 4 {
		wait = acmeCooldownBase << (s.Failures - 1) // 5m, 10m, 20m, 40m, then 1h
	}
	s.Until = now.Add(wait)
	if m := acmeRetryAfter.FindStringSubmatch(cause.Error()); m != nil {
		if t, err := time.Parse(time.DateTime, m[1]); err == nil && t.After(s.Until) {
			s.Until = t
		}
	}
	s.Error = cause.Error()
	if data, err := json.Marshal(s); err == nil {
		_ = file.WriteAtomic(path, data, 0644)
	}
	return s
}

// guardACME runs request unless an earlier failure for this certificate is
// still cooling down. In http mode a busy port 80 fails before contacting the CA.
func guardACME(config *conf.CertConfig, request func() error) error {
	path := acmeCooldownPath(config.CertFile)
	now := time.Now()
	if s := loadACMECooldown(path); now.Before(s.Until) {
		return fmt.Errorf("上次申请证书失败，为避免触发 Let's Encrypt 限流，%s 前不会再次申请；"+
			"问题处理后删除 %s 并重启 V2bX 可立即重试。上次错误：%s",
			s.Until.Local().Format(time.DateTime), path, s.Error)
	}
	if config.CertMode == "http" {
		ln, err := net.Listen("tcp", ":"+httpChallengePort)
		if err != nil {
			return fmt.Errorf("TCP %s 端口不可用，未向 Let's Encrypt 发起申请：%s%s",
				httpChallengePort, err, (&Lego{config: config}).challengeHint(err))
		}
		_ = ln.Close()
	}
	if err := request(); err != nil {
		recordACMEFailure(path, err, now)
		return err
	}
	_ = os.Remove(path)
	return nil
}

func (c *Controller) acmeRequest(op func(*Lego) error) error {
	return guardACME(c.CertConfig, func() error {
		l, err := NewLego(c.CertConfig)
		if err != nil {
			return fmt.Errorf("create lego object error: %s", err)
		}
		return op(l)
	})
}
