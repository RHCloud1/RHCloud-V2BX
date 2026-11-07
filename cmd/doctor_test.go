package cmd

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/InazumaV/V2bX/conf"
)

func TestPanelProbeAndRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/config" || r.URL.Query().Get("token") != "dummy-secret" {
			t.Error("incorrect panel request")
		}
		io.WriteString(w, `{"server_port":8443}`)
	}))
	defer server.Close()
	c := conf.ApiConfig{APIHost: server.URL + "/", Key: "dummy-secret", NodeID: 2, NodeType: "hysteria2"}
	port, err := probePanel(context.Background(), c)
	if err != nil || port != 8443 {
		t.Fatalf("port=%d err=%v", port, err)
	}
	server.Close()
	_, err = probePanel(context.Background(), c)
	if err == nil || strings.Contains(err.Error(), c.Key) || strings.Contains(err.Error(), server.URL) {
		t.Fatalf("unredacted error: %v", err)
	}
}

func TestPanelProbeDoesNotFollowRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/", http.StatusFound)
	}))
	defer server.Close()
	_, err := probePanel(context.Background(), conf.ApiConfig{APIHost: server.URL, Key: "dummy-secret"})
	if err == nil || err.Error() != "HTTP 302" {
		t.Fatalf("unexpected redirect result: %v", err)
	}
}

func TestSocksProbeDistinguishesProviderRejection(t *testing.T) {
	for _, reply := range []byte{0, 2} {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			var header [2]byte
			if _, err = io.ReadFull(conn, header[:]); err != nil {
				return
			}
			methods := make([]byte, int(header[1]))
			if _, err = io.ReadFull(conn, methods); err != nil {
				return
			}
			conn.Write([]byte{5, 0})
			var request [4]byte
			if _, err = io.ReadFull(conn, request[:]); err != nil {
				return
			}
			var addressSize int
			switch request[3] {
			case 1:
				addressSize = 4
			case 4:
				addressSize = 16
			case 3:
				var n [1]byte
				io.ReadFull(conn, n[:])
				addressSize = int(n[0])
			}
			io.ReadFull(conn, make([]byte, addressSize+2))
			conn.Write([]byte{5, reply, 0, 1, 0, 0, 0, 0, 0, 0})
		}()
		host, portText, _ := net.SplitHostPort(listener.Addr().String())
		port, _ := strconv.Atoi(portText)
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		err = probeSocks(ctx, socksOutbound{Server: host, Port: port}, "example.com:443")
		cancel()
		listener.Close()
		<-done
		if reply == 0 && err != nil {
			t.Fatal(err)
		}
		if reply == 2 && (err == nil || !strings.Contains(err.Error(), "code=2")) {
			t.Fatalf("missing provider reply: %v", err)
		}
	}
}
