package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/InazumaV/V2bX/conf"
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

func TestDeviceLimitsDoNotBleedBetweenUsers(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"users":[{"id":1,"uuid":"one","device_limit":3,"alive_ip":1},{"id":2,"uuid":"two","device_limit":0,"alive_ip":0},{"id":3,"uuid":"three","device_limit":4,"alive_ip":0}]}`))
	})
	users, err := c.GetUserList()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 || users[0].DeviceLimit != 2 || users[1].DeviceLimit != 0 || users[2].DeviceLimit != 4 {
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
