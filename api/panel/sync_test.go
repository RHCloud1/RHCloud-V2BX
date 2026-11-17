package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/InazumaV/V2bX/conf"
	"github.com/vmihailenco/msgpack/v5"
)

func fixtureClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	c, err := New(&conf.ApiConfig{APIHost: s.URL, Key: "dummy-secret", NodeType: "anytls", NodeID: 7})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFailedNodeResponsesAreNotCached(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"temporary"}`))
	})
	for i := 0; i < 2; i++ {
		if node, err := c.GetNodeInfo(); err == nil || node != nil {
			t.Fatalf("attempt %d cached a failed response: %v %v", i, node, err)
		}
	}
}

func TestEmptyUserListDiffersFromNotModified(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == "empty" {
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", "empty")
		w.Write([]byte(`{"users":[]}`))
	})
	users, err := c.GetUserList()
	if err != nil || users == nil || len(users) != 0 {
		t.Fatalf("empty update must be non-nil: %v %v", users, err)
	}
	users, err = c.GetUserList()
	if err != nil || users != nil {
		t.Fatalf("304 must be nil: %v %v", users, err)
	}
}

// alive_ip in the user list is ignored; online devices come from alivelist only.
func TestDeviceLimitsDoNotBleedBetweenUsers(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"users":[{"id":1,"uuid":"one","device_limit":3,"alive_ip":1},{"id":2,"uuid":"two","device_limit":0,"alive_ip":0},{"id":3,"uuid":"three","device_limit":4,"alive_ip":0}]}`))
	})
	users, err := c.GetUserList()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 || users[0].DeviceLimit != 3 || users[1].DeviceLimit != 0 || users[2].DeviceLimit != 4 {
		t.Fatalf("incorrect independent limits: %+v", users)
	}
}

func TestPanelErrorDoesNotExposeToken(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403); w.Write([]byte(r.URL.RawQuery)) })
	_, err := c.GetUserList()
	if err == nil || strings.Contains(err.Error(), "dummy-secret") {
		t.Fatalf("unredacted panel error: %v", err)
	}
}

// Xboard's naive payload (ServerService::buildNodeConfig) sends tls as 0/1.
func TestNaiveNodeAlwaysUsesTLS(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"protocol":"naive","listen_ip":"0.0.0.0","server_port":8443,"tls":0,"tls_settings":null,"base_config":{"push_interval":60,"pull_interval":60}}`))
	}))
	t.Cleanup(s.Close)
	c, err := New(&conf.ApiConfig{APIHost: s.URL, Key: "dummy-secret", NodeType: "naive", NodeID: 9})
	if err != nil {
		t.Fatal(err)
	}
	n, err := c.GetNodeInfo()
	if err != nil || n == nil {
		t.Fatalf("node=%v err=%v", n, err)
	}
	if n.Naive == nil || n.Common.ServerPort != 8443 || n.Security != Tls {
		t.Fatalf("naive node = %+v security=%d", n.Naive, n.Security)
	}
}

func TestNodeDefaultsWithoutBaseConfig(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"server_port":443}`)) })
	n, err := c.GetNodeInfo()
	if err != nil || n == nil {
		t.Fatalf("node=%v err=%v", n, err)
	}
	if n.PullInterval != time.Minute || n.PushInterval != time.Minute {
		t.Fatalf("unsafe default intervals: %v %v", n.PullInterval, n.PushInterval)
	}
}

func TestInvalidIntervalsUseSafeDefaults(t *testing.T) {
	for _, value := range []any{nil, 0, -1, "", "NaN", "1e100", []string{"invalid"}} {
		if actual := intervalToTime(value); actual != time.Minute {
			t.Fatalf("interval %v became %v", value, actual)
		}
	}
}

func TestUserListPrefersMsgpack(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Response-Format") != "msgpack" {
			t.Errorf("msgpack not requested")
		}
		body, err := msgpack.Marshal(&UserListBody{Users: []UserInfo{{Id: 5, Uuid: "five", SpeedLimit: 10, DeviceLimit: 2}}})
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/x-msgpack")
		w.Header().Set("ETag", "v1")
		w.Write(body)
	})
	users, err := c.GetUserList()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0] != (UserInfo{Id: 5, Uuid: "five", SpeedLimit: 10, DeviceLimit: 2}) {
		t.Fatalf("users = %+v", users)
	}
	if c.userEtag != "v1" {
		t.Fatalf("etag = %q", c.userEtag)
	}
}

func TestTuicNodeParsed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"server_port":443,"server_name":"tuic.example.com","congestion_control":"bbr","zero_rtt_handshake":true}`))
	}))
	t.Cleanup(s.Close)
	c, err := New(&conf.ApiConfig{APIHost: s.URL, Key: "dummy-secret", NodeType: "tuic", NodeID: 3})
	if err != nil {
		t.Fatal(err)
	}
	n, err := c.GetNodeInfo()
	if err != nil || n == nil {
		t.Fatalf("node=%v err=%v", n, err)
	}
	if n.Tuic == nil || n.Tuic.CongestionControl != "bbr" || !n.Tuic.ZeroRTTHandshake || n.Security != Tls {
		t.Fatalf("tuic node = %+v security=%d", n.Tuic, n.Security)
	}
}

func TestVlessEncryptionParsed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"server_port":443,"network":"tcp","tls":2,"encryption":"mlkem768x25519plus","encryption_settings":{"mode":"native","ticket":"600s","server_padding":"","private_key":"key"},"tls_settings":{"server_name":"a.com","private_key":"pk","mldsa65Seed":"seed"}}`))
	}))
	t.Cleanup(s.Close)
	c, err := New(&conf.ApiConfig{APIHost: s.URL, Key: "dummy-secret", NodeType: "vless", NodeID: 4})
	if err != nil {
		t.Fatal(err)
	}
	n, err := c.GetNodeInfo()
	if err != nil || n == nil {
		t.Fatalf("node=%v err=%v", n, err)
	}
	v := n.VAllss
	if v.Encryption != "mlkem768x25519plus" || v.EncryptionSettings.Mode != "native" || v.EncryptionSettings.PrivateKey != "key" || v.TlsSettings.Mldsa65Seed != "seed" {
		t.Fatalf("vless node = %+v", v)
	}
}

func TestApiSendIPBindsLocalAddress(t *testing.T) {
	var remote string
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		remote = r.RemoteAddr
		w.Write([]byte(`{"users":[]}`))
	})
	bound, err := New(&conf.ApiConfig{APIHost: c.APIHost, APISendIP: "127.0.0.2", Key: "dummy-secret", NodeType: "anytls", NodeID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.GetUserList(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(remote, "127.0.0.2:") {
		t.Fatalf("request came from %q", remote)
	}
}
