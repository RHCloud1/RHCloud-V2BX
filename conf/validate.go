package conf

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	certcheck "github.com/InazumaV/V2bX/common/cert"
)

var coreProtocols = map[string][]string{
	"sing":      {"vmess", "vless", "trojan", "shadowsocks", "hysteria", "hysteria2", "anytls", "naive"},
	"xray":      {"vmess", "vless", "trojan", "shadowsocks"},
	"hysteria2": {"hysteria2"},
}

// Validate performs local checks only. Ports and user credentials come from Xboard.
func (c *Conf) Validate() error {
	if len(c.CoresConfig) == 0 || len(c.NodeConfig) == 0 {
		return fmt.Errorf("at least one core and one node are required")
	}
	cores := make(map[string]string)
	for i, core := range c.CoresConfig {
		if _, ok := coreProtocols[core.Type]; !ok {
			return fmt.Errorf("Cores[%d]: unsupported Type %q", i, core.Type)
		}
		name := core.Name
		if name == "" {
			name = core.Type
		}
		if _, ok := cores[name]; ok {
			return fmt.Errorf("Cores[%d]: duplicate core name %q", i, name)
		}
		cores[name] = core.Type
	}
	ids, names := make(map[string]bool), make(map[string]bool)
	type owner struct{ domain, mode, pair string }
	paths := make(map[string]owner)
	for i, n := range c.NodeConfig {
		if err := validateNode(n, cores); err != nil {
			return fmt.Errorf("Nodes[%d] (NodeID %d): %w", i, n.ApiConfig.NodeID, err)
		}
		protocol := strings.ToLower(n.ApiConfig.NodeType)
		if protocol == "v2ray" {
			protocol = "vmess"
		}
		id := fmt.Sprintf("%s/%s/%d", strings.TrimRight(n.ApiConfig.APIHost, "/"), protocol, n.ApiConfig.NodeID)
		if ids[id] || (n.Options.Name != "" && names[n.Options.Name]) {
			return fmt.Errorf("Nodes[%d]: duplicate node identity or Name", i)
		}
		ids[id], names[n.Options.Name] = true, true
		cc := n.Options.CertConfig
		if cc == nil || cc.CertMode == "" || cc.CertMode == "none" {
			continue
		}
		certPath, _ := filepath.Abs(cc.CertFile)
		keyPath, _ := filepath.Abs(cc.KeyFile)
		pair := certPath + "\x00" + keyPath
		for _, p := range []string{cc.CertFile, cc.KeyFile} {
			p, err := filepath.Abs(p)
			if err != nil {
				return err
			}
			if prev, ok := paths[p]; ok && (!strings.EqualFold(prev.domain, cc.CertDomain) || prev.mode != cc.CertMode) &&
				(cc.CertMode != "file" || prev.mode != "file") {
				return fmt.Errorf("Nodes[%d]: certificate path %q is shared by different domains or modes; use /etc/V2bX/certs/{domain}/...", i, p)
			}
			if prev, ok := paths[p]; ok && prev.pair != pair {
				return fmt.Errorf("Nodes[%d]: certificate/key path %q is reused with a different key pair", i, p)
			}
			paths[p] = owner{cc.CertDomain, cc.CertMode, pair}
		}
	}
	return nil
}

func validateNode(n NodeConfig, cores map[string]string) error {
	u, err := url.Parse(n.ApiConfig.APIHost)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("ApiHost must be an http(s) URL")
	}
	if n.ApiConfig.NodeID <= 0 || n.ApiConfig.Key == "" {
		return fmt.Errorf("positive NodeID and non-empty ApiKey are required")
	}
	protocol := strings.ToLower(n.ApiConfig.NodeType)
	if protocol == "v2ray" {
		protocol = "vmess"
	}
	coreType := n.Options.Core
	if n.Options.CoreName != "" {
		var ok bool
		coreType, ok = cores[n.Options.CoreName]
		if !ok || (n.Options.Core != "" && n.Options.Core != coreType) {
			return fmt.Errorf("CoreName does not identify a configured core matching Core")
		}
	}
	supported := false
	for _, configured := range cores {
		if coreType != "" && configured != coreType {
			continue
		}
		for _, p := range coreProtocols[configured] {
			if protocol == p {
				supported = true
			}
		}
	}
	if !supported {
		return fmt.Errorf("no configured core supports NodeType %q", protocol)
	}
	if n.Options.ListenIP != "" {
		if _, err := netip.ParseAddr(n.Options.ListenIP); err != nil {
			return fmt.Errorf("ListenIP must be an IP address, not a domain")
		}
	} else if coreType != "hysteria2" {
		return fmt.Errorf("ListenIP is required (0.0.0.0 or ::)")
	}
	cc := n.Options.CertConfig
	mode := "none"
	if cc != nil && cc.CertMode != "" {
		mode = cc.CertMode
	}
	if (protocol == "anytls" || protocol == "naive" || protocol == "hysteria" || protocol == "hysteria2") && mode == "none" {
		return fmt.Errorf("%s requires TLS; configure CertConfig", protocol)
	}
	if mode == "none" {
		return nil
	}
	if mode != "file" && mode != "http" && mode != "dns" && mode != "self" {
		return fmt.Errorf("unsupported CertMode %q", mode)
	}
	if cc.CertFile == "" || cc.KeyFile == "" || filepath.Clean(cc.CertFile) == filepath.Clean(cc.KeyFile) {
		return fmt.Errorf("CertFile and KeyFile must be distinct non-empty paths")
	}
	if mode != "file" && cc.CertDomain == "" {
		return fmt.Errorf("CertDomain is required")
	}
	if mode == "dns" && (cc.Provider == "" || len(cc.DNSEnv) == 0) {
		return fmt.Errorf("DNS certificates require Provider and DNSEnv")
	}
	_, certErr := os.Stat(cc.CertFile)
	_, keyErr := os.Stat(cc.KeyFile)
	if mode != "file" && os.IsNotExist(certErr) && os.IsNotExist(keyErr) {
		return nil
	}
	if mode == "http" || mode == "dns" {
		_, err := certcheck.Identity(cc.CertFile, cc.KeyFile, cc.CertDomain)
		return err
	}
	return certcheck.Check(cc.CertFile, cc.KeyFile, cc.CertDomain, time.Now())
}
