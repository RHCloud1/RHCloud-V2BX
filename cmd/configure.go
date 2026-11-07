package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/InazumaV/V2bX/common/file"
	"github.com/InazumaV/V2bX/conf"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type configPrompt struct {
	reader *bufio.Reader
	input  io.Reader
	output io.Writer
}

func (p *configPrompt) ask(label, fallback string, secret bool) (string, error) {
	fmt.Fprint(p.output, label)
	var value string
	if f, ok := p.input.(*os.File); secret && ok && term.IsTerminal(int(f.Fd())) {
		data, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(p.output)
		if err != nil {
			return "", err
		}
		value = string(data)
	} else {
		line, err := p.reader.ReadString('\n')
		if err != nil && (err != io.EOF || len(line) == 0) {
			return "", fmt.Errorf("configuration input ended")
		}
		value = strings.TrimRight(line, "\r\n")
	}
	if value == "" {
		value = fallback
	}
	return value, nil
}

func newConfigureCommand() *cobra.Command {
	var output string
	c := &cobra.Command{Use: "configure", Short: "Generate node and outbound configuration interactively", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return configure(cmd.InOrStdin(), cmd.OutOrStdout(), output)
		}}
	c.Flags().StringVarP(&output, "config", "c", "/etc/V2bX/config.json", "output configuration path")
	return c
}

func configure(input io.Reader, output io.Writer, path string) error {
	p := &configPrompt{bufio.NewReader(input), input, output}
	fmt.Fprintln(output, "V2bX 配置向导：生成同一 Xboard 面板的多个节点；现有文件会备份。")
	answer, err := p.ask("继续？[y/N]：", "n", false)
	if err != nil {
		return err
	}
	if !strings.EqualFold(answer, "y") {
		return nil
	}
	host, err := p.ask("Xboard 地址：https://...：", "", false)
	if err != nil {
		return err
	}
	key, err := p.ask("面板 API Key（不回显）：", "", true)
	if err != nil {
		return err
	}
	var nodes, cores []map[string]any
	files := map[string][]byte{}
	dir := filepath.Dir(path)
	for i := 1; ; i++ {
		core, err := p.ask("核心：1.Xray 2.sing-box 3.Hysteria2 [2]：", "2", false)
		if err != nil {
			return err
		}
		coreTypes := map[string]string{"1": "xray", "2": "sing", "3": "hysteria2"}
		coreType, ok := coreTypes[core]
		if !ok {
			return fmt.Errorf("invalid core selection")
		}
		idText, err := p.ask("Xboard Node ID：", "", false)
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(idText)
		if err != nil || id <= 0 {
			return fmt.Errorf("Node ID must be positive")
		}
		protocol := "hysteria2"
		if coreType != "hysteria2" {
			choice, err := p.ask("协议：1.Shadowsocks 2.VLESS 3.VMess 4.Hysteria 5.Hysteria2 6.Trojan 7.AnyTLS 8.Naive：", "", false)
			if err != nil {
				return err
			}
			protocols := map[string]string{"1": "shadowsocks", "2": "vless", "3": "vmess", "4": "hysteria", "5": "hysteria2", "6": "trojan", "7": "anytls", "8": "naive"}
			protocol, ok = protocols[choice]
			if !ok {
				return fmt.Errorf("invalid protocol selection")
			}
			if coreType == "xray" && (protocol == "anytls" || protocol == "naive" || strings.HasPrefix(protocol, "hysteria")) {
				return fmt.Errorf("Xray does not support this protocol")
			}
		}
		listen, err := p.ask("监听 IP [0.0.0.0]，可填 :: 或本机具体 IPv6：", "0.0.0.0", false)
		if err != nil {
			return err
		}
		if _, err = netip.ParseAddr(listen); err != nil {
			return fmt.Errorf("invalid listen IP")
		}
		cert := conf.NewCertConfig()
		tlsRequired := protocol == "anytls" || protocol == "naive" || protocol == "trojan" || strings.HasPrefix(protocol, "hysteria")
		if !tlsRequired {
			answer, err = p.ask("是否使用 TLS 证书？[y/N]（Reality 选 n）：", "n", false)
			if err != nil {
				return err
			}
			tlsRequired = strings.EqualFold(answer, "y")
		}
		if tlsRequired {
			choice, err := p.ask("证书模式：1.HTTP 2.DNS 3.自签 4.已有文件 [1]：", "1", false)
			if err != nil {
				return err
			}
			modes := map[string]string{"1": "http", "2": "dns", "3": "self", "4": "file"}
			cert.CertMode, ok = modes[choice]
			if !ok {
				return fmt.Errorf("invalid certificate mode")
			}
			if protocol == "naive" && cert.CertMode == "self" {
				fmt.Fprintln(p.output, "注意：NaiveProxy 官方客户端不接受自签证书，自签只适合能关闭证书校验的客户端测试。")
			}
			cert.CertDomain, err = p.ask("证书域名/SNI：", "", false)
			if err != nil {
				return err
			}
			if cert.CertDomain == "" || strings.ContainsAny(cert.CertDomain, "/\\") {
				return fmt.Errorf("invalid certificate domain")
			}
			cert.CertFile = filepath.Join(dir, "certs", "{domain}", "fullchain.cer")
			cert.KeyFile = filepath.Join(dir, "certs", "{domain}", "cert.key")
			if cert.CertMode == "file" {
				cert.CertFile, err = p.ask("证书链文件完整路径：", "", false)
				if err != nil {
					return err
				}
				cert.KeyFile, err = p.ask("私钥文件完整路径：", "", false)
				if err != nil {
					return err
				}
			} else if cert.CertMode != "self" {
				cert.Email, err = p.ask("ACME 邮箱：", "", false)
				if err != nil {
					return err
				}
				if !strings.Contains(cert.Email, "@") {
					return fmt.Errorf("ACME email is required")
				}
			}
			if cert.CertMode == "dns" {
				cert.Provider, err = p.ask("DNS 提供商 [cloudflare]：", "cloudflare", false)
				if err != nil {
					return err
				}
				env, err := p.ask("DNS 环境变量 JSON（不回显）：", "", true)
				if err != nil {
					return err
				}
				if json.Unmarshal([]byte(env), &cert.DNSEnv) != nil || len(cert.DNSEnv) == 0 {
					return fmt.Errorf("invalid DNS environment JSON")
				}
			}
		}
		coreName := fmt.Sprintf("%s-node-%d", coreType, i)
		cc := map[string]any{"Type": coreType, "Name": coreName, "Log": map[string]any{"Level": "error"}}
		if coreType == "sing" {
			answer, err = p.ask("此节点全部走住宅 SOCKS 出口？[y/N]：", "n", false)
			if err != nil {
				return err
			}
			residential := strings.EqualFold(answer, "y")
			outbound := map[string]any{"type": "direct", "tag": "direct"}
			final := "direct"
			suffix := "direct"
			if residential {
				server, err := p.ask("SOCKS 服务器：", "", false)
				if err != nil {
					return err
				}
				portText, err := p.ask("SOCKS 端口 [10001]：", "10001", false)
				if err != nil {
					return err
				}
				port, err := strconv.Atoi(portText)
				if err != nil || port < 1 || port > 65535 {
					return fmt.Errorf("invalid SOCKS port")
				}
				username, err := p.ask("SOCKS 用户名：", "", false)
				if err != nil {
					return err
				}
				password, err := p.ask("SOCKS 密码（不回显）：", "", true)
				if err != nil {
					return err
				}
				if server == "" || username == "" || password == "" {
					return fmt.Errorf("SOCKS server and credentials are required")
				}
				outbound = map[string]any{"type": "socks", "tag": "residential", "server": server, "server_port": port, "version": "5", "username": username, "password": password, "network": "tcp"}
				final = "residential"
				suffix = "residential"
			}
			origin := filepath.Join(dir, fmt.Sprintf("sing-%s-%d.json", suffix, i))
			data, err := json.MarshalIndent(map[string]any{
				"dns":       map[string]any{"servers": []map[string]any{{"type": "local", "tag": "local"}}},
				"outbounds": []map[string]any{outbound},
				"route":     map[string]any{"rules": []map[string]any{{"ip_is_private": true, "action": "reject"}}, "final": final, "default_domain_resolver": "local"},
			}, "", "  ")
			if err != nil {
				return err
			}
			files[origin] = data
			cc["OriginalPath"] = origin
		}
		cores = append(cores, cc)
		node := map[string]any{"Core": coreType, "CoreName": coreName, "ApiHost": host, "ApiKey": key, "NodeID": id, "NodeType": protocol, "ListenIP": listen, "CertConfig": cert}
		if protocol == "naive" {
			answer, err = p.ask("Naive 同时启用 HTTP/3（额外占用同端口 UDP）？[y/N]：", "n", false)
			if err != nil {
				return err
			}
			node["NaiveQUIC"] = strings.EqualFold(answer, "y")
		}
		nodes = append(nodes, node)
		answer, err = p.ask("继续添加节点？[y/N]：", "n", false)
		if err != nil {
			return err
		}
		if !strings.EqualFold(answer, "y") {
			break
		}
	}
	data, err := json.MarshalIndent(map[string]any{"Log": map[string]any{"Level": "info"}, "Cores": cores, "Nodes": nodes}, "", "  ")
	if err != nil {
		return err
	}
	var parsed conf.Conf
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("generated configuration could not be decoded")
	}
	if err := parsed.Validate(); err != nil {
		return err
	}
	if err := checkCompiledCoreTypes(&parsed); err != nil {
		return err
	}
	files[path] = data
	if err := writeConfigBundle(files, path); err != nil {
		return err
	}
	fmt.Fprintf(output, "已生成 %d 个节点：%s。已有文件已备份；可运行 V2bX check 和 V2bX restart。\n", len(nodes), path)
	return nil
}

func writeConfigBundle(files map[string][]byte, mainPath string) error {
	type snapshot struct {
		path    string
		data    []byte
		existed bool
	}
	var written []snapshot
	paths := make([]string, 0, len(files))
	for path := range files {
		if path != mainPath {
			paths = append(paths, path)
		}
	}
	paths = append(paths, mainPath)
	sort.Strings(paths[:len(paths)-1])
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	var snapshots []snapshot
	for _, path := range paths {
		previous, err := os.ReadFile(path)
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if exists {
			if err = file.WriteAtomic(path+".bak."+stamp, previous, 0600); err != nil {
				return err
			}
		}
		snapshots = append(snapshots, snapshot{path, previous, exists})
	}
	for _, s := range snapshots {
		if err := file.WriteAtomic(s.path, files[s.path], 0600); err != nil {
			rollbackErrors := []error{err}
			for i := len(written) - 1; i >= 0; i-- {
				s := written[i]
				if s.existed {
					rollbackErrors = append(rollbackErrors, file.WriteAtomic(s.path, s.data, 0600))
				} else {
					rollbackErrors = append(rollbackErrors, os.Remove(s.path))
				}
			}
			return errors.Join(rollbackErrors...)
		}
		written = append(written, s)
	}
	return nil
}

func init() { command.AddCommand(newConfigureCommand()) }
