//go:build sing && hysteria2 && with_quic

package node

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	_ "github.com/InazumaV/V2bX/core/hy2"
	_ "github.com/InazumaV/V2bX/core/sing"
	"github.com/InazumaV/V2bX/limiter"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/json/badoption"
	M "github.com/sagernet/sing/common/metadata"
)

// This is an isolated Xboard-compatible fixture, not a claim of real Xboard acceptance.
type simulatedPanel struct {
	mu       sync.Mutex
	configs  map[int]map[string]any
	users    map[int][]panel.UserInfo
	traffic  map[int]map[int][2]int64
	failPush bool
}

func (p *simulatedPanel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.URL.Query().Get("token") != "simulation-token" {
		w.WriteHeader(403)
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("node_id"))
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/v1/server/UniProxy/config":
		data, _ := json.Marshal(p.configs[id])
		tag := fmt.Sprintf("%x", sha256.Sum256(data))
		w.Header().Set("ETag", tag)
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(304)
			return
		}
		w.Write(data)
	case "/api/v1/server/UniProxy/user":
		data, _ := json.Marshal(map[string]any{"users": p.users[id]})
		tag := fmt.Sprintf("%x", sha256.Sum256(data))
		w.Header().Set("ETag", tag)
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(304)
			return
		}
		w.Write(data)
	case "/api/v1/server/UniProxy/alivelist":
		io.WriteString(w, `{"alive":{}}`)
	case "/api/v1/server/UniProxy/push":
		if p.failPush {
			w.WriteHeader(503)
			io.WriteString(w, `{"error":"simulated outage"}`)
			return
		}
		var usage map[int][2]int64
		if json.NewDecoder(r.Body).Decode(&usage) != nil {
			w.WriteHeader(400)
			return
		}
		if p.traffic[id] == nil {
			p.traffic[id] = make(map[int][2]int64)
		}
		for uid, value := range usage {
			total := p.traffic[id][uid]
			total[0] += value[0]
			total[1] += value[1]
			p.traffic[id][uid] = total
		}
		io.WriteString(w, `{"data":true}`)
	case "/api/v1/server/UniProxy/alive":
		io.WriteString(w, `{"data":true}`)
	default:
		w.WriteHeader(404)
	}
}

func simulationPort(t *testing.T, host, network string) int {
	t.Helper()
	if network == "tcp" {
		l, err := net.Listen(network, net.JoinHostPort(host, "0"))
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	l, err := net.ListenPacket(network, net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port
}

func simulationBox(t *testing.T, opts option.Options) *box.Box {
	t.Helper()
	ctx := box.Context(context.Background(), include.InboundRegistry(), include.OutboundRegistry(), include.EndpointRegistry(), include.DNSTransportRegistry(), include.ServiceRegistry(), include.CertificateProviderRegistry())
	opts.Log = &option.LogOptions{Disabled: true}
	b, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		b.Close()
		t.Fatal(err)
	}
	return b
}

func simulationRequest(t *testing.T, protocol, host string, port int, password, sni string, cert []byte, target string) error {
	t.Helper()
	server := option.ServerOptions{Server: host, ServerPort: uint16(port)}
	tls := option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{Enabled: true, ServerName: sni, Certificate: []string{string(cert)}}}
	var opts any = &option.AnyTLSOutboundOptions{ServerOptions: server, Password: password, OutboundTLSOptionsContainer: tls}
	if protocol == "hysteria2" {
		opts = &option.Hysteria2OutboundOptions{ServerOptions: server, Password: password, OutboundTLSOptionsContainer: tls}
	}
	b := simulationBox(t, option.Options{Outbounds: []option.Outbound{{Type: protocol, Tag: "proxy", Options: opts}}})
	defer b.Close()
	out, _ := b.Outbound().Outbound("proxy")
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return out.DialContext(ctx, network, M.ParseSocksaddr(address))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 2 * time.Second, Transport: transport}
	response, err := client.Post(target, "text/plain", strings.NewReader(strings.Repeat("upload", 1024)))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if string(data) != strings.Repeat("download", 1024) {
		return fmt.Errorf("unexpected response")
	}
	return nil
}

func TestSimulatedXboardNativeHY2AndResidentialAnyTLS(t *testing.T) {
	limiter.Init()
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			if host == "::1" {
				l, err := net.Listen("tcp", "[::1]:0")
				if err != nil {
					t.Skip("IPv6 loopback unavailable:", err)
				}
				l.Close()
			}
			dir := t.TempDir()
			certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
			if err := generateSelfSslCertificate("node.example", certPath, keyPath); err != nil {
				t.Fatal(err)
			}
			cert, err := os.ReadFile(certPath)
			if err != nil {
				t.Fatal(err)
			}
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				if err != nil || r.Method != http.MethodPost || string(data) != strings.Repeat("upload", 1024) {
					http.Error(w, "invalid upload payload", http.StatusBadRequest)
					return
				}
				io.WriteString(w, strings.Repeat("download", 1024))
			}))
			defer target.Close()
			gatewayPort := simulationPort(t, "127.0.0.1", "tcp")
			addr := badoption.Addr(M.ParseAddr("127.0.0.1"))
			gateway := simulationBox(t, option.Options{Inbounds: []option.Inbound{{Type: "socks", Tag: "gateway", Options: &option.SocksInboundOptions{ListenOptions: option.ListenOptions{Listen: &addr, ListenPort: uint16(gatewayPort)}, Users: []auth.User{{Username: "simulation-user", Password: "simulation-password"}}}}}})
			defer gateway.Close()
			original := filepath.Join(dir, "sing-residential.json")
			originalData, _ := json.Marshal(map[string]any{"outbounds": []any{map[string]any{"type": "socks", "tag": "residential", "server": "127.0.0.1", "server_port": gatewayPort, "version": "5", "username": "simulation-user", "password": "simulation-password", "network": "tcp"}}, "route": map[string]any{"final": "residential"}})
			if err := os.WriteFile(original, originalData, 0600); err != nil {
				t.Fatal(err)
			}
			anyPort, hyPort := simulationPort(t, host, "tcp"), simulationPort(t, host, "udp")
			user := panel.UserInfo{Id: 1001, Uuid: "11111111-1111-4111-8111-111111111111"}
			fixture := &simulatedPanel{configs: map[int]map[string]any{}, users: map[int][]panel.UserInfo{2: {user}, 7: {user}}, traffic: map[int]map[int][2]int64{}}
			fixture.configs[2] = map[string]any{"server_port": hyPort, "base_config": map[string]int{"pull_interval": 3600, "push_interval": 3600}}
			fixture.configs[7] = map[string]any{"server_port": anyPort, "base_config": map[string]int{"pull_interval": 3600, "push_interval": 3600}}
			panelServer := httptest.NewServer(fixture)
			defer panelServer.Close()
			cores := []conf.CoreConfig{{Type: "hysteria2", Name: "hy", Hysteria2Config: conf.NewHysteria2Config()}, {Type: "sing", Name: "any", SingConfig: conf.NewSingConfig()}}
			cores[1].SingConfig.OriginalPath = original
			configs := []conf.NodeConfig{}
			for _, spec := range []struct {
				id                   int
				protocol, core, name string
			}{{2, "hysteria2", "hysteria2", "hy"}, {7, "anytls", "sing", "any"}} {
				configs = append(configs, conf.NodeConfig{ApiConfig: conf.ApiConfig{APIHost: panelServer.URL, Key: "simulation-token", NodeID: spec.id, NodeType: spec.protocol, Timeout: 1}, Options: conf.Options{Core: spec.core, CoreName: spec.name, ListenIP: host, SingOptions: conf.NewSingOptions(), CertConfig: &conf.CertConfig{CertMode: "file", CertDomain: "node.example", CertFile: certPath, KeyFile: keyPath}}})
			}
			start := func() (vCore.Core, *Node) {
				vc, err := vCore.NewCore(cores)
				if err != nil {
					t.Fatal(err)
				}
				if err := vc.Start(); err != nil {
					vc.Close()
					t.Fatal(err)
				}
				nodes := New()
				if err := nodes.Start(configs, vc); err != nil {
					vc.Close()
					t.Fatal(err)
				}
				return vc, nodes
			}
			vc, nodes := start()
			defer func() { nodes.Close(); vc.Close() }()
			request := func(protocol string, port int, password string) error {
				return simulationRequest(t, protocol, host, port, password, "node.example", cert, target.URL)
			}
			if err := request("anytls", anyPort, user.Uuid); err != nil {
				t.Fatal("AnyTLS through residential gateway:", err)
			}
			if err := request("hysteria2", hyPort, user.Uuid); err != nil {
				t.Fatal("native Hysteria2:", err)
			}
			if err := request("anytls", anyPort, "wrong-password"); err == nil {
				t.Fatal("invalid credentials accepted")
			}
			if err := simulationRequest(t, "anytls", host, anyPort, user.Uuid, "wrong.example", cert, target.URL); err == nil {
				t.Fatal("wrong certificate SNI accepted")
			}
			for _, controller := range nodes.controllers {
				if err := controller.reportUserTrafficTask(); err != nil {
					t.Fatal(err)
				}
			}
			fixture.mu.Lock()
			for _, id := range []int{2, 7} {
				usage := fixture.traffic[id][user.Id]
				if usage[0] <= 0 || usage[1] <= 0 {
					t.Errorf("node %d did not report both traffic directions: %v", id, usage)
				}
			}
			fixture.failPush = true
			fixture.mu.Unlock()
			if err := request("anytls", anyPort, user.Uuid); err != nil {
				t.Fatal(err)
			}
			anyController := nodes.controllers[1]
			anyController.reportUserTrafficTask()
			if len(anyController.pendingTraffic) == 0 {
				t.Fatal("failed push lost usage")
			}
			fixture.mu.Lock()
			fixture.failPush = false
			fixture.mu.Unlock()
			anyController.reportUserTrafficTask()
			if len(anyController.pendingTraffic) != 0 {
				t.Fatal("successful retry did not acknowledge pending usage")
			}
			fixture.mu.Lock()
			fixture.users[7] = []panel.UserInfo{}
			fixture.mu.Unlock()
			if err := anyController.nodeInfoMonitor(); err != nil {
				t.Fatal(err)
			}
			if err := request("anytls", anyPort, user.Uuid); err == nil {
				t.Fatal("revoked last user still connected")
			}
			if err := request("hysteria2", hyPort, user.Uuid); err != nil {
				t.Fatal("AnyTLS revocation affected HY2:", err)
			}
			newUser := panel.UserInfo{Id: 1002, Uuid: "22222222-2222-4222-8222-222222222222"}
			fixture.mu.Lock()
			fixture.users[7] = []panel.UserInfo{newUser}
			fixture.mu.Unlock()
			if err := anyController.nodeInfoMonitor(); err != nil {
				t.Fatal(err)
			}
			if err := request("anytls", anyPort, newUser.Uuid); err != nil {
				t.Fatal("restored user cannot connect:", err)
			}
			if err := request("anytls", anyPort, user.Uuid); err == nil {
				t.Fatal("removed credential accepted after restore")
			}
			busy, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
			if err != nil {
				t.Fatal(err)
			}
			defer busy.Close()
			newPort := busy.Addr().(*net.TCPAddr).Port
			fixture.mu.Lock()
			fixture.configs[7]["server_port"] = newPort
			fixture.configs[7]["base_config"] = map[string]int{"pull_interval": 3601, "push_interval": 3602}
			fixture.mu.Unlock()
			if err := anyController.nodeInfoMonitor(); err != nil {
				t.Fatal(err)
			}
			if err := request("anytls", anyPort, newUser.Uuid); err != nil {
				t.Fatal("failed panel rebind did not restore last working listener:", err)
			}
			busy.Close()
			if err := anyController.nodeInfoMonitor(); err != nil {
				t.Fatal(err)
			}
			if err := request("anytls", newPort, newUser.Uuid); err != nil {
				t.Fatal("node did not retry rejected panel config:", err)
			}
			anyPort = newPort
			if anyController.nodeInfoMonitorPeriodic.Interval != 3601*time.Second || anyController.userReportPeriodic.Interval != 3602*time.Second {
				t.Fatal("pull/push intervals did not update independently")
			}
			if err := gateway.Close(); err != nil {
				t.Fatal(err)
			}
			if err := request("anytls", anyPort, newUser.Uuid); err == nil {
				t.Fatal("residential failure fell back to direct")
			}
			if err := request("hysteria2", hyPort, user.Uuid); err != nil {
				t.Fatal("gateway failure affected direct HY2:", err)
			}
			if err := nodes.Close(); err != nil {
				t.Fatal(err)
			}
			if err := vc.Close(); err != nil {
				t.Fatal(err)
			}
			for _, failure := range []string{"second-node-bind", "missing-native-config"} {
				badConfigs := append([]conf.NodeConfig(nil), configs...)
				if failure == "second-node-bind" {
					badConfigs[1].Options.ListenIP = "198.51.100.1"
				} else {
					badConfigs[0].Options.Hysteria2ConfigPath = filepath.Join(dir, "missing-hy2.yaml")
				}
				badCore, err := vCore.NewCore(cores)
				if err != nil {
					t.Fatal(err)
				}
				if err := badCore.Start(); err != nil {
					badCore.Close()
					t.Fatal(err)
				}
				partial := New()
				err = partial.Start(badConfigs, badCore)
				if err == nil {
					partial.Close()
					badCore.Close()
					t.Fatal("expected startup rejection:", failure)
				}
				if len(partial.controllers) != 0 {
					t.Error("partial startup retained live controllers")
				}
				if err := badCore.Close(); err != nil {
					t.Fatal(err)
				}
			}
			vc, nodes = start()
			if err := request("hysteria2", hyPort, user.Uuid); err != nil {
				t.Fatal("restart failed to restore direct node:", err)
			}
			t.Log("PASS: Xboard fixture -> native HY2 + sing-box AnyTLS; traffic, revocation, restore, TLS verification, fail-closed routing and restart")
		})
	}
}
