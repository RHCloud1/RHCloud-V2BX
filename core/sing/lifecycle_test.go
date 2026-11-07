//go:build with_quic

package sing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"github.com/InazumaV/V2bX/core"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
)

func testTLSFiles(t *testing.T) *conf.CertConfig {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "node.example"}, DNSNames: []string{"node.example"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cert := &conf.CertConfig{CertMode: "file", CertDomain: "node.example", CertFile: filepath.Join(dir, "cert.pem"), KeyFile: filepath.Join(dir, "key.pem")}
	if err := os.WriteFile(cert.CertFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cert.KeyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestAnyTLSAndHysteria2DynamicLifecycle(t *testing.T) {
	for _, samePort := range []bool{false, true} {
		t.Run(fmt.Sprintf("shared-numeric-port=%v", samePort), func(t *testing.T) {
			c, err := New(&conf.CoreConfig{Type: "sing", SingConfig: conf.NewSingConfig()})
			if err != nil {
				t.Fatal(err)
			}
			b := c.(*Sing)
			if err := b.Start(); err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			cert := testTLSFiles(t)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			tcpPort := listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			udpPort := tcpPort
			if !samePort {
				packet, err := net.ListenPacket("udp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				udpPort = packet.LocalAddr().(*net.UDPAddr).Port
				packet.Close()
			}
			users := []panel.UserInfo{{Uuid: "dummy-node-password"}}
			for _, protocol := range []string{"anytls", "hysteria2"} {
				port := tcpPort
				if protocol == "hysteria2" {
					port = udpPort
				}
				info := &panel.NodeInfo{Type: protocol, Security: panel.Tls, Common: &panel.CommonNode{ServerPort: port}, AnyTls: &panel.AnyTlsNode{}, Hysteria2: &panel.Hysteria2Node{}}
				if err := b.AddNode(protocol, info, &conf.Options{ListenIP: "127.0.0.1", SingOptions: conf.NewSingOptions(), CertConfig: cert}); err != nil {
					t.Fatal(err)
				}
				if _, err := b.AddUsers(&core.AddUsersParams{Tag: protocol, Users: users}); err != nil {
					t.Fatal(err)
				}
			}
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "both-protocols-work") }))
			defer target.Close()
			certPEM, err := os.ReadFile(cert.CertFile)
			if err != nil {
				t.Fatal(err)
			}
			for _, protocol := range []string{"anytls", "hysteria2"} {
				port := tcpPort
				if protocol == "hysteria2" {
					port = udpPort
				}
				tlsOptions := option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{Enabled: true, ServerName: "node.example", Certificate: []string{string(certPEM)}}}
				server := option.ServerOptions{Server: "127.0.0.1", ServerPort: uint16(port)}
				var opts any = &option.AnyTLSOutboundOptions{ServerOptions: server, Password: users[0].Uuid, OutboundTLSOptionsContainer: tlsOptions}
				if protocol == "hysteria2" {
					opts = &option.Hysteria2OutboundOptions{ServerOptions: server, Password: users[0].Uuid, OutboundTLSOptionsContainer: tlsOptions}
				}
				logger := b.logFactory.NewLogger("test-client")
				scope := adapter.NewScope(b.ctx, logger)
				out, err := include.OutboundRegistry().CreateOutbound(scope.Context(), b.router, logger, "test-"+protocol, protocol, opts)
				if err != nil {
					scope.Close()
					t.Fatal(err)
				}
				for _, stage := range adapter.ListStartStages {
					if err := scope.Start("client", out.(adapter.Lifecycle), stage); err != nil {
						scope.Close()
						t.Fatal(err)
					}
				}
				client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					return out.DialContext(ctx, network, M.ParseSocksaddr(address))
				}}}
				response, err := client.Get(target.URL)
				if err != nil {
					scope.Close()
					t.Fatalf("%s round trip: %v", protocol, err)
				}
				data, err := io.ReadAll(response.Body)
				response.Body.Close()
				client.CloseIdleConnections()
				scope.Close()
				if err != nil || string(data) != "both-protocols-work" {
					t.Fatalf("%s response: %q %v", protocol, data, err)
				}
			}
			if err := b.DelNode("anytls"); err != nil {
				t.Fatal(err)
			}
			listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
			if err != nil {
				t.Fatal("AnyTLS did not release TCP port:", err)
			}
			listener.Close()
			if err := b.DelNode("hysteria2"); err != nil {
				t.Fatal(err)
			}
			packet, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", udpPort))
			if err != nil {
				t.Fatal("HY2 did not release UDP port:", err)
			}
			packet.Close()
		})
	}
}

func TestOriginalPathLegacyConfigRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, []byte(`{"dns":{"servers":[{"address":"local"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readOriginal(path); err == nil || !strings.Contains(err.Error(), "migrate-sing") {
		t.Fatalf("missing actionable migration error: %v", err)
	}
}
