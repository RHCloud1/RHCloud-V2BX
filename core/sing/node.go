package sing

import (
	"encoding/base64"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"github.com/goccy/go-json"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/auth"
	F "github.com/sagernet/sing/common/format"
	"github.com/sagernet/sing/common/json/badoption"
	N "github.com/sagernet/sing/common/network"
)

type HttpNetworkConfig struct {
	Header struct {
		Type     string           `json:"type"`
		Request  *json.RawMessage `json:"request"`
		Response *json.RawMessage `json:"response"`
	} `json:"header"`
}

type HttpRequest struct {
	Version string   `json:"version"`
	Method  string   `json:"method"`
	Path    []string `json:"path"`
	Headers struct {
		Host []string `json:"Host"`
	} `json:"headers"`
}

type WsNetworkConfig struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type GrpcNetworkConfig struct {
	ServiceName string `json:"serviceName"`
}

type HttpupgradeNetworkConfig struct {
	Path string `json:"path"`
	Host string `json:"host"`
}

func getInboundOptions(tag string, info *panel.NodeInfo, c *conf.Options, users []panel.UserInfo) (option.Inbound, error) {
	addr, err := netip.ParseAddr(c.ListenIP)
	if err != nil {
		return option.Inbound{}, fmt.Errorf("the listen ip not vail")
	}
	listen := option.ListenOptions{
		Listen:      (*badoption.Addr)(&addr),
		ListenPort:  uint16(info.Common.ServerPort),
		TCPFastOpen: c.SingOptions.TCPFastOpen,
	}
	var multiplex *option.InboundMultiplexOptions
	if c.SingOptions.Multiplex != nil {
		multiplexOption := option.InboundMultiplexOptions{
			Enabled: c.SingOptions.Multiplex.Enabled,
			Padding: c.SingOptions.Multiplex.Padding,
			Brutal: &option.BrutalOptions{
				Enabled:  c.SingOptions.Multiplex.Brutal.Enabled,
				UpMbps:   c.SingOptions.Multiplex.Brutal.UpMbps,
				DownMbps: c.SingOptions.Multiplex.Brutal.DownMbps,
			},
		}
		multiplex = &multiplexOption
	}
	var tls option.InboundTLSOptions
	switch info.Security {
	case panel.Tls:
		if c.CertConfig == nil {
			return option.Inbound{}, fmt.Errorf("the CertConfig is not vail")
		}
		switch c.CertConfig.CertMode {
		case "none", "":
			break // disable
		default:
			tls.Enabled = true
			tls.CertificatePath = c.CertConfig.CertFile
			tls.KeyPath = c.CertConfig.KeyFile
		}
	case panel.Reality:
		tls.Enabled = true
		v := info.VAllss
		tls.ServerName = v.TlsSettings.ServerName
		port, _ := strconv.Atoi(v.TlsSettings.ServerPort)
		var dest string
		if v.TlsSettings.Dest != "" {
			dest = v.TlsSettings.Dest
		} else {
			dest = tls.ServerName
		}

		mtd, _ := time.ParseDuration(v.RealityConfig.MaxTimeDiff)
		tls.Reality = &option.InboundRealityOptions{
			Enabled:    true,
			ShortID:    []string{v.TlsSettings.ShortId},
			PrivateKey: v.TlsSettings.PrivateKey,
			Handshake: option.InboundRealityHandshakeOptions{
				ServerOptions: option.ServerOptions{
					Server:     dest,
					ServerPort: uint16(port),
				},
			},
			MaxTimeDifference: badoption.Duration(mtd),
		}
	}
	in := option.Inbound{
		Tag: tag,
	}
	switch info.Type {
	case "vmess", "vless":
		n := info.VAllss
		t := option.V2RayTransportOptions{
			Type: n.Network,
		}
		switch n.Network {
		case "tcp":
			if len(n.NetworkSettings) != 0 {
				network := HttpNetworkConfig{}
				err := json.Unmarshal(n.NetworkSettings, &network)
				if err != nil {
					return option.Inbound{}, fmt.Errorf("decode NetworkSettings error: %s", err)
				}
				//Todo fix http options
				if network.Header.Type == "http" {
					t.Type = network.Header.Type
					var request HttpRequest
					if network.Header.Request != nil {
						err = json.Unmarshal(*network.Header.Request, &request)
						if err != nil {
							return option.Inbound{}, fmt.Errorf("decode HttpRequest error: %s", err)
						}
						t.HTTPOptions.Host = request.Headers.Host
						t.HTTPOptions.Path = request.Path[0]
						t.HTTPOptions.Method = request.Method
					}
				} else {
					t.Type = ""
				}
			} else {
				t.Type = ""
			}
		case "ws":
			var (
				path    string
				ed      int
				headers map[string]badoption.Listable[string]
			)
			if len(n.NetworkSettings) != 0 {
				network := WsNetworkConfig{}
				err := json.Unmarshal(n.NetworkSettings, &network)
				if err != nil {
					return option.Inbound{}, fmt.Errorf("decode NetworkSettings error: %s", err)
				}
				var u *url.URL
				u, err = url.Parse(network.Path)
				if err != nil {
					return option.Inbound{}, fmt.Errorf("parse path error: %s", err)
				}
				path = u.Path
				ed, _ = strconv.Atoi(u.Query().Get("ed"))
				headers = make(map[string]badoption.Listable[string], len(network.Headers))
				for k, v := range network.Headers {
					headers[k] = badoption.Listable[string]{
						v,
					}
				}
			}
			t.WebsocketOptions = option.V2RayWebsocketOptions{
				Path:                path,
				EarlyDataHeaderName: "Sec-WebSocket-Protocol",
				MaxEarlyData:        uint32(ed),
				Headers:             headers,
			}
		case "grpc":
			network := GrpcNetworkConfig{}
			if len(n.NetworkSettings) != 0 {
				err := json.Unmarshal(n.NetworkSettings, &network)
				if err != nil {
					return option.Inbound{}, fmt.Errorf("decode NetworkSettings error: %s", err)
				}
			}
			t.GRPCOptions = option.V2RayGRPCOptions{
				ServiceName: network.ServiceName,
			}
		case "httpupgrade":
			network := HttpupgradeNetworkConfig{}
			if len(n.NetworkSettings) != 0 {
				err := json.Unmarshal(n.NetworkSettings, &network)
				if err != nil {
					return option.Inbound{}, fmt.Errorf("decode NetworkSettings error: %s", err)
				}
			}
			t.HTTPUpgradeOptions = option.V2RayHTTPUpgradeOptions{
				Path: network.Path,
				Host: network.Host,
			}
		}
		if info.Type == "vless" {
			in.Type = "vless"
			in.Options = &option.VLESSInboundOptions{
				ListenOptions: listen,
				Users:         buildVLESSUsers(users, n.Flow),
				InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
					TLS: &tls,
				},
				Transport: &t,
				Multiplex: multiplex,
			}
		} else {
			in.Type = "vmess"
			in.Options = &option.VMessInboundOptions{
				ListenOptions: listen,
				Users:         buildVMessUsers(users),
				InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
					TLS: &tls,
				},
				Transport: &t,
				Multiplex: multiplex,
			}
		}
	case "shadowsocks":
		in.Type = "shadowsocks"
		n := info.Shadowsocks
		var keyLength int
		switch n.Cipher {
		case "2022-blake3-aes-128-gcm":
			keyLength = 16
		case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
			keyLength = 32
		default:
			keyLength = 16
		}
		ssoption := &option.ShadowsocksInboundOptions{
			ListenOptions: listen,
			Method:        n.Cipher,
			Multiplex:     multiplex,
		}
		if strings.Contains(n.Cipher, "2022") {
			ssoption.Password = n.ServerKey
		}
		ssoption.Users = buildShadowsocksUsers(users, n.Cipher, keyLength)
		in.Options = ssoption
	case "trojan":
		n := info.Trojan
		t := option.V2RayTransportOptions{
			Type: n.Network,
		}
		switch n.Network {
		case "tcp":
			t.Type = ""
		case "ws":
			var (
				path    string
				ed      int
				headers map[string]badoption.Listable[string]
			)
			if len(n.NetworkSettings) != 0 {
				network := WsNetworkConfig{}
				err := json.Unmarshal(n.NetworkSettings, &network)
				if err != nil {
					return option.Inbound{}, fmt.Errorf("decode NetworkSettings error: %s", err)
				}
				var u *url.URL
				u, err = url.Parse(network.Path)
				if err != nil {
					return option.Inbound{}, fmt.Errorf("parse path error: %s", err)
				}
				path = u.Path
				ed, _ = strconv.Atoi(u.Query().Get("ed"))
				headers = make(map[string]badoption.Listable[string], len(network.Headers))
				for k, v := range network.Headers {
					headers[k] = badoption.Listable[string]{
						v,
					}
				}
			}
			t.WebsocketOptions = option.V2RayWebsocketOptions{
				Path:                path,
				EarlyDataHeaderName: "Sec-WebSocket-Protocol",
				MaxEarlyData:        uint32(ed),
				Headers:             headers,
			}
		case "grpc":
			network := GrpcNetworkConfig{}
			if len(n.NetworkSettings) != 0 {
				err := json.Unmarshal(n.NetworkSettings, &network)
				if err != nil {
					return option.Inbound{}, fmt.Errorf("decode NetworkSettings error: %s", err)
				}
			}
			t.GRPCOptions = option.V2RayGRPCOptions{
				ServiceName: network.ServiceName,
			}
		default:
			t.Type = ""
		}
		in.Type = "trojan"
		trojanoption := &option.TrojanInboundOptions{
			ListenOptions: listen,
			Users:         buildTrojanUsers(users),
			InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
				TLS: &tls,
			},
			Transport: &t,
			Multiplex: multiplex,
		}
		if c.SingOptions.FallBackConfigs != nil {
			// fallback handling
			fallback := c.SingOptions.FallBackConfigs.FallBack
			fallbackPort, err := strconv.Atoi(fallback.ServerPort)
			if err == nil {
				trojanoption.Fallback = &option.ServerOptions{
					Server:     fallback.Server,
					ServerPort: uint16(fallbackPort),
				}
			}
			fallbackForALPNMap := c.SingOptions.FallBackConfigs.FallBackForALPN
			fallbackForALPN := make(map[string]*option.ServerOptions, len(fallbackForALPNMap))
			if err := processFallback(c, fallbackForALPN); err == nil {
				trojanoption.FallbackForALPN = fallbackForALPN
			}
		}
		in.Options = trojanoption
	case "hysteria":
		in.Type = "hysteria"
		in.Options = &option.HysteriaInboundOptions{
			ListenOptions: listen,
			UpMbps:        info.Hysteria.UpMbps,
			DownMbps:      info.Hysteria.DownMbps,
			Obfs:          info.Hysteria.Obfs,
			Users:         buildHysteriaUsers(users),
			InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
				TLS: &tls,
			},
		}
	case "hysteria2":
		in.Type = "hysteria2"
		var obfs *option.Hysteria2Obfs
		if info.Hysteria2.ObfsType != "" && info.Hysteria2.ObfsPassword != "" {
			obfs = &option.Hysteria2Obfs{
				Type:     info.Hysteria2.ObfsType,
				Password: info.Hysteria2.ObfsPassword,
			}
		} else if info.Hysteria2.ObfsType != "" {
			obfs = &option.Hysteria2Obfs{
				Type:     "salamander",
				Password: info.Hysteria2.ObfsType,
			}
		}
		in.Options = &option.Hysteria2InboundOptions{
			ListenOptions: listen,
			UpMbps:        info.Hysteria2.UpMbps,
			DownMbps:      info.Hysteria2.DownMbps,
			Users:         buildHysteria2Users(users),
			Obfs:          obfs,
			InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
				TLS: &tls,
			},
		}
	case "anytls":
		if !tls.Enabled {
			return option.Inbound{}, fmt.Errorf("anytls requires TLS certificate configuration")
		}
		in.Type = "anytls"
		in.Options = &option.AnyTLSInboundOptions{
			ListenOptions: listen,
			Users:         buildAnyTLSUsers(users),
			PaddingScheme: badoption.Listable[string](info.AnyTls.PaddingScheme),
			InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
				TLS: &tls,
			},
		}
	case "naive":
		if !tls.Enabled {
			return option.Inbound{}, fmt.Errorf("naive requires TLS certificate configuration")
		}
		// HTTP/2 over TCP by default; HTTP/3 also binds UDP on the same port.
		network := option.NetworkList(N.NetworkTCP)
		if c.SingOptions.NaiveQUIC {
			network = option.NetworkList(N.NetworkTCP + "\n" + N.NetworkUDP)
		}
		in.Type = "naive"
		in.Options = &option.NaiveInboundOptions{
			ListenOptions: listen,
			Users:         buildNaiveUsers(users),
			Network:       network,
			InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
				TLS: &tls,
			},
		}
	default:
		return option.Inbound{}, fmt.Errorf("unsupported node type %q for sing core", info.Type)
	}
	return in, nil
}

func (b *Sing) AddNode(tag string, info *panel.NodeInfo, config *conf.Options) error {
	b.nodesMu.Lock()
	defer b.nodesMu.Unlock()
	if _, found := b.nodes[tag]; found {
		return fmt.Errorf("duplicate node tag %q", tag)
	}
	if _, found := b.box.Inbound().Get(tag); found {
		return fmt.Errorf("node tag %q conflicts with OriginalPath inbound", tag)
	}
	b.nodes[tag] = &nodeState{
		info:   info,
		config: config,
	}
	return nil
}

func (b *Sing) createInbound(tag string, info *panel.NodeInfo, config *conf.Options, users []panel.UserInfo) error {
	c, err := getInboundOptions(tag, info, config, users)
	if err != nil {
		return err
	}
	logger := b.logFactory.NewLogger(F.ToString("inbound/", c.Type, "[", tag, "]"))
	scope := adapter.NewScope(b.ctx, logger)
	in, err := include.InboundRegistry().Create(
		scope.Context(),
		b.box.Router(),
		logger,
		tag,
		c.Type,
		c.Options,
	)

	if err != nil {
		scope.Close()
		return fmt.Errorf("add inbound error: %w", err)
	}
	// Since sing-box 1.14, the manager only creates static inbounds. Own each
	// dynamic listener's lifecycle scope so updates and removal release its port.
	for _, stage := range adapter.ListStartStages {
		if err := scope.Start(tag, in, stage); err != nil {
			scope.Close()
			return fmt.Errorf("start inbound: %w", err)
		}
	}
	b.nodes[tag].inbound, b.nodes[tag].scope = in, scope
	return nil
}

func (b *Sing) DelNode(tag string) error {
	b.nodesMu.Lock()
	defer b.nodesMu.Unlock()
	if state := b.nodes[tag]; state != nil && state.scope != nil {
		if err := state.scope.Close(); err != nil {
			return fmt.Errorf("delete inbound: %w", err)
		}
	}
	delete(b.nodes, tag)
	b.hookServer.counter.Delete(tag)
	return nil
}

func (b *Sing) recreateInbound(tag string) error {
	state, found := b.nodes[tag]
	if !found {
		return fmt.Errorf("node state not found")
	}
	if state.scope != nil {
		err := state.scope.Close()
		state.scope, state.inbound = nil, nil
		if err != nil {
			return fmt.Errorf("close inbound: %w", err)
		}
	}
	return b.createInbound(tag, state.info, state.config, state.users)
}

func buildVMessUsers(users []panel.UserInfo) []option.VMessUser {
	result := make([]option.VMessUser, len(users))
	for i := range users {
		result[i] = option.VMessUser{
			Name: users[i].Uuid,
			UUID: users[i].Uuid,
		}
	}
	return result
}

func buildVLESSUsers(users []panel.UserInfo, flow string) []option.VLESSUser {
	result := make([]option.VLESSUser, len(users))
	for i := range users {
		result[i] = option.VLESSUser{
			Name: users[i].Uuid,
			Flow: flow,
			UUID: users[i].Uuid,
		}
	}
	return result
}

func buildShadowsocksUsers(users []panel.UserInfo, cipher string, keyLength int) []option.ShadowsocksUser {
	result := make([]option.ShadowsocksUser, len(users))
	for i := range users {
		password := users[i].Uuid
		if strings.Contains(cipher, "2022") && len(password) >= keyLength {
			password = base64.StdEncoding.EncodeToString([]byte(password[:keyLength]))
		}
		result[i] = option.ShadowsocksUser{
			Name:     users[i].Uuid,
			Password: password,
		}
	}
	return result
}

func buildTrojanUsers(users []panel.UserInfo) []option.TrojanUser {
	result := make([]option.TrojanUser, len(users))
	for i := range users {
		result[i] = option.TrojanUser{
			Name:     users[i].Uuid,
			Password: users[i].Uuid,
		}
	}
	return result
}

func buildHysteriaUsers(users []panel.UserInfo) []option.HysteriaUser {
	result := make([]option.HysteriaUser, len(users))
	for i := range users {
		result[i] = option.HysteriaUser{
			Name:       users[i].Uuid,
			AuthString: users[i].Uuid,
		}
	}
	return result
}

func buildHysteria2Users(users []panel.UserInfo) []option.Hysteria2User {
	result := make([]option.Hysteria2User, len(users))
	for i := range users {
		result[i] = option.Hysteria2User{
			Name:     users[i].Uuid,
			Password: users[i].Uuid,
		}
	}
	return result
}

// Xboard's socks/http/naive nodes use the user's UUID as both username and
// password; the username also keys the traffic counter.
func buildNaiveUsers(users []panel.UserInfo) []auth.User {
	result := make([]auth.User, len(users))
	for i := range users {
		result[i] = auth.User{
			Username: users[i].Uuid,
			Password: users[i].Uuid,
		}
	}
	return result
}

func buildAnyTLSUsers(users []panel.UserInfo) []option.AnyTLSUser {
	result := make([]option.AnyTLSUser, len(users))
	for i := range users {
		result[i] = option.AnyTLSUser{
			Name:     users[i].Uuid,
			Password: users[i].Uuid,
		}
	}
	return result
}
