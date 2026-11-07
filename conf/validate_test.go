package conf

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func testConfig(t *testing.T) *Conf {
	t.Helper()
	dir := t.TempDir()
	c := New()
	c.CoresConfig = []CoreConfig{{Type: "sing"}, {Type: "hysteria2"}}
	for i, protocol := range []string{"anytls", "hysteria2"} {
		c.NodeConfig = append(c.NodeConfig, NodeConfig{
			ApiConfig: ApiConfig{APIHost: "https://panel.example.com", Key: "test-key", NodeID: i + 1, NodeType: protocol},
			Options: Options{Core: c.CoresConfig[i].Type, ListenIP: "::", CertConfig: &CertConfig{
				CertMode: "http", CertDomain: "node.example.com", CertFile: filepath.Join(dir, "fullchain.cer"), KeyFile: filepath.Join(dir, "cert.key"),
			}},
		})
	}
	return c
}

func TestValidateMultiNode(t *testing.T) {
	if err := testConfig(t).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsBrokenConfiguration(t *testing.T) {
	cases := map[string]func(*Conf){
		"empty cores":         func(c *Conf) { c.CoresConfig = nil },
		"duplicate core":      func(c *Conf) { c.CoresConfig = append(c.CoresConfig, c.CoresConfig[0]) },
		"unknown core":        func(c *Conf) { c.CoresConfig[0].Type = "other" },
		"wrong protocol":      func(c *Conf) { c.NodeConfig[0].Options.Core = "hysteria2" },
		"unknown core name":   func(c *Conf) { c.NodeConfig[0].Options.CoreName = "missing" },
		"invalid listen":      func(c *Conf) { c.NodeConfig[0].Options.ListenIP = "example.com" },
		"missing TLS":         func(c *Conf) { c.NodeConfig[0].Options.CertConfig = nil },
		"shared cert domains": func(c *Conf) { c.NodeConfig[0].Options.CertConfig.CertDomain = "other.example.com" },
		"same cert key path": func(c *Conf) {
			c.NodeConfig[0].Options.CertConfig.KeyFile = c.NodeConfig[0].Options.CertConfig.CertFile
		},
		"duplicate node":  func(c *Conf) { c.NodeConfig = append(c.NodeConfig, c.NodeConfig[0]) },
		"duplicate name":  func(c *Conf) { c.NodeConfig[0].Options.Name = "same"; c.NodeConfig[1].Options.Name = "same" },
		"missing API key": func(c *Conf) { c.NodeConfig[0].ApiConfig.Key = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := testConfig(t)
			mutate(c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestValidateNaive(t *testing.T) {
	c := testConfig(t)
	c.NodeConfig[0].ApiConfig.NodeType = "naive"
	if err := c.Validate(); err != nil {
		t.Fatalf("naive on sing: %v", err)
	}
	c.NodeConfig[0].Options.CertConfig = nil
	if err := c.Validate(); err == nil {
		t.Fatal("naive without TLS must fail")
	}
	c = testConfig(t)
	c.CoresConfig[0].Type = "xray"
	c.NodeConfig[0].Options.Core = "xray"
	c.NodeConfig[0].ApiConfig.NodeType = "naive"
	if err := c.Validate(); err == nil {
		t.Fatal("naive on xray must fail")
	}
}

func TestCertificatePathTemplates(t *testing.T) {
	var c CertConfig
	if err := json.Unmarshal([]byte(`{"CertDomain":"any.example.com","CertFile":"/etc/V2bX/certs/{domain}/fullchain.cer","KeyFile":"/etc/V2bX/certs/{domain}/cert.key"}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.CertFile != "/etc/V2bX/certs/any.example.com/fullchain.cer" || c.KeyFile != "/etc/V2bX/certs/any.example.com/cert.key" {
		t.Fatalf("unresolved paths: %+v", c)
	}
	if err := json.Unmarshal([]byte(`{"CertDomain":"../other","CertFile":"/{domain}/cert"}`), &c); err == nil {
		t.Fatal("unsafe domain accepted")
	}
}
