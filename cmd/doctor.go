package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	certcheck "github.com/InazumaV/V2bX/common/cert"
	"github.com/InazumaV/V2bX/conf"
	"github.com/spf13/cobra"
	"golang.org/x/net/proxy"
)

func newDoctorCommand() *cobra.Command {
	var configPath, target string
	var network bool
	c := &cobra.Command{Use: "doctor", Short: "Inspect local configuration, certificates and listeners; opt into network probes with --network", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := conf.New()
			if err := c.LoadFromPath(configPath); err != nil {
				return err
			}
			failures := 0
			report := func(label string, err error) {
				if err != nil {
					failures++
					fmt.Fprintf(cmd.OutOrStdout(), "FAIL %s: %v\n", label, err)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "OK   %s\n", label)
				}
			}
			report("local configuration", c.Validate())
			report("compiled cores and original configurations", checkCompiledCores(c))
			for i, n := range c.NodeConfig {
				fmt.Fprintf(cmd.OutOrStdout(), "Node %d: %s, ListenIP=%s\n", n.ApiConfig.NodeID, n.ApiConfig.NodeType, n.Options.ListenIP)
				cc := n.Options.CertConfig
				if cc != nil && cc.CertMode != "" && cc.CertMode != "none" {
					leaf, err := certcheck.Identity(cc.CertFile, cc.KeyFile, cc.CertDomain)
					if errors.Is(err, os.ErrNotExist) && cc.CertMode != "file" {
						fmt.Fprintln(cmd.OutOrStdout(), "PENDING certificate will be created at service startup")
					} else if err != nil {
						report("certificate identity", err)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "Certificate %s expires %s (%d days remaining)\n", cc.CertDomain, leaf.NotAfter.UTC().Format(time.RFC3339), int(time.Until(leaf.NotAfter).Hours()/24))
						report("certificate validity", certcheck.Check(cc.CertFile, cc.KeyFile, cc.CertDomain, time.Now()))
					}
				}
				if network {
					ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Second)
					port, err := probePanel(ctx, n.ApiConfig)
					cancel()
					report(fmt.Sprintf("node[%d] Xboard config endpoint", i), err)
					if err == nil {
						transport := "tcp"
						if n.ApiConfig.NodeType == "hysteria2" || n.ApiConfig.NodeType == "hysteria" {
							transport = "udp"
						}
						fmt.Fprintf(cmd.OutOrStdout(), "Panel server_port=%d/%s (compare with ss output below)\n", port, transport)
					}
					if cc != nil && cc.CertDomain != "" {
						ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
						ips, err := net.DefaultResolver.LookupHost(ctx, cc.CertDomain)
						cancel()
						report("DNS "+cc.CertDomain, err)
						if err == nil {
							fmt.Fprintf(cmd.OutOrStdout(), "Addresses: %v\n", ips)
						}
					}
				}
			}
			if network {
				for _, cc := range c.CoresConfig {
					if cc.Type != "sing" || cc.SingConfig == nil || cc.SingConfig.OriginalPath == "" {
						continue
					}
					outbounds, err := readSocksOutbounds(cc.SingConfig.OriginalPath)
					if err != nil {
						report("read sing-box outbounds", err)
						continue
					}
					for _, o := range outbounds {
						ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Second)
						err := probeSocks(ctx, o, target)
						cancel()
						report("SOCKS "+o.Tag+" TCP CONNECT", err)
					}
				}
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Network probes skipped. Use --network to check Xboard, DNS and SOCKS TCP CONNECT.")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()
			if data, err := exec.CommandContext(ctx, "ss", "-lntup").Output(); err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Listener listing unavailable: install iproute2 / run ss -lntup on the VPS.")
			}
			if failures > 0 {
				return fmt.Errorf("%d diagnostic checks failed", failures)
			}
			return nil
		}}
	c.Flags().StringVarP(&configPath, "config", "c", "/etc/V2bX/config.json", "configuration path")
	c.Flags().BoolVar(&network, "network", false, "contact the configured Xboard/DNS/SOCKS endpoints")
	c.Flags().StringVar(&target, "target", "www.gstatic.com:443", "SOCKS CONNECT target; no application data is sent")
	return c
}

func probePanel(ctx context.Context, c conf.ApiConfig) (int, error) {
	u, err := url.Parse(c.APIHost)
	if err != nil {
		return 0, fmt.Errorf("invalid panel URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/server/UniProxy/config"
	q := u.Query()
	q.Set("node_type", c.NodeType)
	q.Set("node_id", strconv.Itoa(c.NodeID))
	q.Set("token", c.Key)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("invalid panel request")
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("panel connection failed (check TLS, DNS and connectivity)")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	var node struct {
		Port int `json:"server_port"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&node) != nil || node.Port < 1 || node.Port > 65535 {
		return 0, fmt.Errorf("panel response has no valid server_port")
	}
	return node.Port, nil
}

type socksOutbound struct {
	Type     string `json:"type"`
	Tag      string `json:"tag"`
	Server   string `json:"server"`
	Port     int    `json:"server_port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func readSocksOutbounds(path string) ([]socksOutbound, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config struct {
		Outbounds []socksOutbound `json:"outbounds"`
	}
	if json.Unmarshal(data, &config) != nil {
		return nil, fmt.Errorf("invalid original configuration JSON")
	}
	var result []socksOutbound
	for _, o := range config.Outbounds {
		if o.Type == "socks" {
			result = append(result, o)
		}
	}
	return result, nil
}
func probeSocks(ctx context.Context, o socksOutbound, target string) error {
	auth := (*proxy.Auth)(nil)
	if o.Username != "" {
		auth = &proxy.Auth{User: o.Username, Password: o.Password}
	}
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort(o.Server, strconv.Itoa(o.Port)), auth, &net.Dialer{Timeout: 8 * time.Second})
	if err != nil {
		return fmt.Errorf("invalid SOCKS settings")
	}
	conn, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", target)
	if err != nil {
		if strings.Contains(err.Error(), "connection not allowed by ruleset") {
			return fmt.Errorf("SOCKS5 reply code=2: gateway rejected the CONNECT request (provider ruleset)")
		}
		return fmt.Errorf("SOCKS CONNECT failed; inspect gateway availability, credentials and provider restrictions")
	}
	return conn.Close()
}
func init() { command.AddCommand(newDoctorCommand()) }
