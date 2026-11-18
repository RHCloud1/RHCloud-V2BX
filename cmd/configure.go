package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

// defaultACMEEmail matches the address the original shell wizard wrote, so
// existing lego account files (user-<email>.json) keep working.
const defaultACMEEmail = "v2bx@github.com"

func (p *configPrompt) yes(label string) (bool, error) {
	answer, err := p.ask(label, "", false)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), err
}

// choose prints a numbered menu in the original wizard's style and asks
// again until one of the listed options is entered.
func (p *configPrompt) choose(title string, options [][2]string) (string, error) {
	for {
		fmt.Fprintln(p.output, title)
		for _, o := range options {
			fmt.Fprintf(p.output, "%s. %s\n", o[0], o[1])
		}
		answer, err := p.ask("请输入：", "", false)
		if err != nil {
			return "", err
		}
		for _, o := range options {
			if answer == o[0] {
				return answer, nil
			}
		}
		fmt.Fprintln(p.output, "无效的选择，请重新输入。")
	}
}

func hasIPv6() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() == nil && n.IP.IsGlobalUnicast() {
			return true
		}
	}
	return false
}

func configure(input io.Reader, output io.Writer, path string) error {
	p := &configPrompt{bufio.NewReader(input), input, output}
	fmt.Fprintln(output, "V2bX 配置文件生成向导")
	fmt.Fprintln(output, "请阅读以下注意事项：")
	fmt.Fprintf(output, "1. 生成的配置文件会保存到 %s\n", path)
	fmt.Fprintln(output, "2. 原来的配置文件会备份为 config.json.bak.时间")
	fmt.Fprintln(output, "3. 确定继续？(y/n)")
	answer, err := p.ask("请输入：", "", false)
	if err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(answer), "n") {
		return nil
	}
	var host, key string
	askPanel := func(hostLabel string) error {
		if host, err = p.ask(hostLabel, "", false); err != nil {
			return err
		}
		key, err = p.ask("请输入面板对接API Key：", "", false)
		return err
	}
	if err = askPanel("请输入机场网址："); err != nil {
		return err
	}
	fixedPanel, err := p.yes("是否设置固定的机场网址和API Key？(y/n)")
	if err != nil {
		return err
	}
	if fixedPanel {
		fmt.Fprintln(output, "成功固定地址")
	}
	var nodes, cores []map[string]any
	files := map[string][]byte{}
	dir := filepath.Dir(path)
	for i := 1; ; i++ {
		if i > 1 {
			answer, err = p.ask("是否继续添加节点配置？(回车继续，输入n或no退出)", "", false)
			if err != nil {
				return err
			}
			if strings.HasPrefix(strings.ToLower(answer), "n") {
				break
			}
			if !fixedPanel {
				if err = askPanel("请输入机场网址(https://example.com)："); err != nil {
					return err
				}
			}
		}
		core, err := p.choose("请选择节点核心类型：", [][2]string{{"1", "xray"}, {"2", "singbox"}, {"3", "hysteria2"}})
		if err != nil {
			return err
		}
		coreType := map[string]string{"1": "xray", "2": "sing", "3": "hysteria2"}[core]
		var id int
		for {
			idText, err := p.ask("请输入节点Node ID：", "", false)
			if err != nil {
				return err
			}
			if id, err = strconv.Atoi(idText); err == nil && id > 0 {
				break
			}
			fmt.Fprintln(output, "错误：请输入正确的数字作为Node ID。")
		}
		protocol := "hysteria2"
		if coreType != "hysteria2" {
			options := [][2]string{{"1", "Shadowsocks"}, {"2", "Vless"}, {"3", "Vmess"}}
			if coreType == "sing" {
				options = append(options, [2]string{"4", "Hysteria"}, [2]string{"5", "Hysteria2"})
			}
			options = append(options, [2]string{"6", "Trojan"})
			if coreType == "sing" {
				options = append(options, [2]string{"7", "AnyTLS"}, [2]string{"8", "Naive"}, [2]string{"9", "TUIC"})
			}
			choice, err := p.choose("请选择节点传输协议：", options)
			if err != nil {
				return err
			}
			protocol = map[string]string{"1": "shadowsocks", "2": "vless", "3": "vmess", "4": "hysteria", "5": "hysteria2", "6": "trojan", "7": "anytls", "8": "naive", "9": "tuic"}[choice]
		}
		cert := conf.NewCertConfig()
		tlsRequired := protocol == "anytls" || protocol == "naive" || protocol == "tuic" || protocol == "trojan" || strings.HasPrefix(protocol, "hysteria")
		if !tlsRequired {
			reality := false
			if protocol == "vless" {
				if reality, err = p.yes("请选择是否为reality节点？(y/n)"); err != nil {
					return err
				}
			}
			if !reality {
				if tlsRequired, err = p.yes("请选择是否进行TLS配置？(y/n)"); err != nil {
					return err
				}
			}
		}
		if tlsRequired {
			choice, err := p.choose("请选择证书申请模式：", [][2]string{
				{"1", "http模式自动申请，节点域名已正确解析"},
				{"2", "dns模式自动申请，需填入正确域名服务商API参数"},
				{"3", "self模式，自签证书或提供已有证书文件"},
			})
			if err != nil {
				return err
			}
			cert.CertMode = map[string]string{"1": "http", "2": "dns", "3": "self"}[choice]
			if protocol == "naive" && cert.CertMode == "self" {
				fmt.Fprintln(output, "注意：NaiveProxy 官方客户端不接受自签证书，自签只适合能关闭证书校验的客户端测试。")
			}
			for {
				if cert.CertDomain, err = p.ask("请输入节点证书域名(example.com)：", "", false); err != nil {
					return err
				}
				if cert.CertDomain != "" && !strings.ContainsAny(cert.CertDomain, "/\\") {
					break
				}
				fmt.Fprintln(output, "错误：请输入正确的域名。")
			}
			cert.CertFile = filepath.Join(dir, "certs", "{domain}", "fullchain.cer")
			cert.KeyFile = filepath.Join(dir, "certs", "{domain}", "cert.key")
			cert.Email = defaultACMEEmail
			switch cert.CertMode {
			case "dns":
				if cert.Provider, err = p.ask("请输入DNS服务商(回车默认cloudflare)：", "cloudflare", false); err != nil {
					return err
				}
				for {
					env, err := p.ask(`请输入DNS API参数，JSON格式，如 {"CF_DNS_API_TOKEN":"xxx"}（不回显）：`, "", true)
					if err != nil {
						return err
					}
					if json.Unmarshal([]byte(env), &cert.DNSEnv) == nil && len(cert.DNSEnv) > 0 {
						break
					}
					fmt.Fprintln(output, "错误：DNS API参数格式不正确。")
				}
			case "self":
				certDir := filepath.Join(dir, "certs", cert.CertDomain)
				fmt.Fprintf(output, "已有证书请放到 %s/fullchain.cer 和 %s/cert.key，否则自动生成自签证书。\n", certDir, certDir)
			}
		}
		listen := map[string]string{"xray": "0.0.0.0", "sing": "0.0.0.0", "hysteria2": ""}[coreType]
		if coreType == "sing" && hasIPv6() {
			listen = "::"
		}
		if coreType != "hysteria2" {
			for {
				if listen, err = p.ask(fmt.Sprintf("请输入监听IP(回车默认%s)：", listen), listen, false); err != nil {
					return err
				}
				if _, err = netip.ParseAddr(listen); err == nil {
					break
				}
				fmt.Fprintln(output, "错误：请输入正确的IP，如 0.0.0.0 或 ::。")
				listen = "0.0.0.0"
			}
		}
		coreName := fmt.Sprintf("%s-node-%d", coreType, i)
		cc := map[string]any{"Type": coreType, "Name": coreName, "Log": map[string]any{"Level": "error"}}
		if coreType == "sing" {
			residential, err := p.yes("是否让此节点全部走住宅SOCKS出口？(y/n，回车默认否)")
			if err != nil {
				return err
			}
			outbound := map[string]any{"type": "direct", "tag": "direct"}
			final := "direct"
			suffix := "direct"
			if residential {
				server, err := p.ask("请输入SOCKS服务器地址：", "", false)
				if err != nil {
					return err
				}
				portText, err := p.ask("请输入SOCKS端口(回车默认10001)：", "10001", false)
				if err != nil {
					return err
				}
				port, err := strconv.Atoi(portText)
				if err != nil || port < 1 || port > 65535 {
					return fmt.Errorf("invalid SOCKS port")
				}
				username, err := p.ask("请输入SOCKS用户名：", "", false)
				if err != nil {
					return err
				}
				password, err := p.ask("请输入SOCKS密码（不回显）：", "", true)
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
			quic, err := p.yes("Naive是否同时启用HTTP/3（额外占用同端口UDP）？(y/n)")
			if err != nil {
				return err
			}
			node["NaiveQUIC"] = quic
		}
		nodes = append(nodes, node)
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
	fmt.Fprintf(output, "V2bX 配置文件生成完成，共 %d 个节点：%s\n", len(nodes), path)
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
