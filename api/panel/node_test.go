package panel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "token" {
			t.Error("missing API token")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			w.Write([]byte(`{"server_port":443,"network":"tcp","base_config":{"pull_interval":60,"push_interval":60}}`))
		} else {
			w.Write([]byte(`{"data":true}`))
		}
	}))
	t.Cleanup(s.Close)
	c, err := New(&conf.ApiConfig{
		APIHost:  s.URL,
		Key:      "token",
		NodeType: "V2ray",
		NodeID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClient_GetNodeInfo(t *testing.T) {
	client := testClient(t)
	n, err := client.GetNodeInfo()
	if err != nil || n == nil || n.Common.ServerPort != 443 {
		t.Fatalf("node=%v err=%v", n, err)
	}
	n, err = client.GetNodeInfo()
	if err != nil || n != nil {
		t.Fatalf("unchanged response should return nil: node=%v err=%v", n, err)
	}
}

func TestClient_ReportUserTraffic(t *testing.T) {
	client := testClient(t)
	if err := client.ReportUserTraffic([]UserTraffic{
		{
			UID:      10372,
			Upload:   1000,
			Download: 1000,
		},
	}); err != nil {
		t.Fatal(err)
	}
}
