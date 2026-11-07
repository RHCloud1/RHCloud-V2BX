//go:build with_quic

package sing

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"github.com/InazumaV/V2bX/core"
	"github.com/sagernet/sing-box/option"
)

func naiveNodeInfo(port int) *panel.NodeInfo {
	return &panel.NodeInfo{
		Type:     "naive",
		Security: panel.Tls,
		Common:   &panel.CommonNode{ServerPort: port},
		Naive:    &panel.NaiveNode{CommonNode: panel.CommonNode{ServerPort: port}},
	}
}

func TestGetNaiveInboundOptions(t *testing.T) {
	for _, quic := range []bool{false, true} {
		t.Run(fmt.Sprintf("quic=%v", quic), func(t *testing.T) {
			options := conf.NewSingOptions()
			options.NaiveQUIC = quic
			config := &conf.Options{ListenIP: "0.0.0.0", SingOptions: options, CertConfig: &conf.CertConfig{CertMode: "file", CertFile: "/etc/V2bX/fullchain.cer", KeyFile: "/etc/V2bX/cert.key"}}
			users := []panel.UserInfo{{Id: 1, Uuid: "user-uuid"}}
			inbound, err := getInboundOptions("node-naive", naiveNodeInfo(443), config, users)
			if err != nil {
				t.Fatal(err)
			}
			naive, ok := inbound.Options.(*option.NaiveInboundOptions)
			if inbound.Type != "naive" || !ok {
				t.Fatalf("inbound = %q %T", inbound.Type, inbound.Options)
			}
			if len(naive.Users) != 1 || naive.Users[0].Username != "user-uuid" || naive.Users[0].Password != "user-uuid" {
				t.Fatalf("users = %#v", naive.Users)
			}
			if naive.TLS == nil || !naive.TLS.Enabled {
				t.Fatal("naive TLS is not enabled")
			}
			want := []string{"tcp"}
			if quic {
				want = []string{"tcp", "udp"}
			}
			if got := naive.Network.Build(); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("network = %v, want %v", got, want)
			}
		})
	}
}

func TestGetNaiveInboundOptionsRequiresTLS(t *testing.T) {
	config := &conf.Options{ListenIP: "0.0.0.0", SingOptions: conf.NewSingOptions(), CertConfig: conf.NewCertConfig()}
	if _, err := getInboundOptions("node-naive", naiveNodeInfo(443), config, nil); err == nil {
		t.Fatal("expected naive without a certificate to fail")
	}
}

// Starts a real naive listener and checks UUID authentication. Data relay after
// CONNECT uses naive's padding framing and is covered by sing-box itself.
func TestNaiveListenerAuthenticatesPanelUsers(t *testing.T) {
	c, err := New(&conf.CoreConfig{Type: "sing", SingConfig: conf.NewSingConfig()})
	if err != nil {
		t.Fatal(err)
	}
	b := c.(*Sing)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	config := &conf.Options{ListenIP: "127.0.0.1", SingOptions: conf.NewSingOptions(), CertConfig: testTLSFiles(t)}
	if err := b.AddNode("naive-test", naiveNodeInfo(port), config); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddUsers(&core.AddUsersParams{Tag: "naive-test", Users: []panel.UserInfo{{Id: 1, Uuid: "good-uuid"}}}); err != nil {
		t.Fatal(err)
	}

	// Over HTTP/1.1 naive answers a bad login by closing the connection with no
	// response (probe resistance), so "rejected" means no HTTP status at all.
	const rejected = 0
	connect := func(user string) int {
		t.Helper()
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{ServerName: "node.example", InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}})
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + user))
		fmt.Fprintf(conn, "CONNECT 127.0.0.1:9 HTTP/1.1\r\nHost: 127.0.0.1:9\r\nProxy-Authorization: Basic %s\r\nPadding: %s\r\n\r\n", auth, "0000000000000000")
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				t.Fatalf("naive did not answer: %v", err)
			}
			return rejected
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := connect("wrong-uuid"); code != rejected {
		t.Fatalf("wrong user status = %d, want connection closed", code)
	}
	if code := connect("good-uuid"); code != http.StatusOK {
		t.Fatalf("panel user status = %d, want 200", code)
	}

	// Revoking the user on the panel must lock it out after the inbound rebuild.
	if err := b.DelUsers([]panel.UserInfo{{Id: 1, Uuid: "good-uuid"}}, "naive-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddUsers(&core.AddUsersParams{Tag: "naive-test", Users: []panel.UserInfo{{Id: 2, Uuid: "next-uuid"}}}); err != nil {
		t.Fatal(err)
	}
	if code := connect("good-uuid"); code != rejected {
		t.Fatalf("revoked user status = %d, want connection closed", code)
	}
	if code := connect("next-uuid"); code != http.StatusOK {
		t.Fatalf("new user status = %d, want 200", code)
	}
}
