package panel

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/InazumaV/V2bX/common/crypt"
	"github.com/goccy/go-json"
)

// Security type
const (
	None    = 0
	Tls     = 1
	Reality = 2
)

type NodeInfo struct {
	Id           int
	Type         string
	Security     int
	PushInterval time.Duration
	PullInterval time.Duration
	RawDNS       RawDNS
	Rules        Rules

	// origin
	VAllss      *VAllssNode
	Shadowsocks *ShadowsocksNode
	Trojan      *TrojanNode
	Hysteria    *HysteriaNode
	Hysteria2   *Hysteria2Node
	AnyTls      *AnyTlsNode
	Naive       *NaiveNode
	Common      *CommonNode
}

type CommonNode struct {
	Host       string      `json:"host"`
	ServerPort int         `json:"server_port"`
	ServerName string      `json:"server_name"`
	Routes     []Route     `json:"routes"`
	BaseConfig *BaseConfig `json:"base_config"`
}

type Route struct {
	Id          int         `json:"id"`
	Match       interface{} `json:"match"`
	Action      string      `json:"action"`
	ActionValue string      `json:"action_value"`
}
type BaseConfig struct {
	PushInterval any `json:"push_interval"`
	PullInterval any `json:"pull_interval"`
}

// VAllssNode is vmess and vless node info
type VAllssNode struct {
	CommonNode
	Tls                 int             `json:"tls"`
	TlsSettings         TlsSettings     `json:"tls_settings"`
	TlsSettingsBack     *TlsSettings    `json:"tlsSettings"`
	Network             string          `json:"network"`
	NetworkSettings     json.RawMessage `json:"network_settings"`
	NetworkSettingsBack json.RawMessage `json:"networkSettings"`
	ServerName          string          `json:"server_name"`

	// vless only
	Flow          string        `json:"flow"`
	RealityConfig RealityConfig `json:"-"`
}

type TlsSettings struct {
	ServerName string `json:"server_name"`
	Dest       string `json:"dest"`
	ServerPort string `json:"server_port"`
	ShortId    string `json:"short_id"`
	PrivateKey string `json:"private_key"`
	Xver       uint64 `json:"xver,string"`
}

type RealityConfig struct {
	Xver         uint64 `json:"Xver"`
	MinClientVer string `json:"MinClientVer"`
	MaxClientVer string `json:"MaxClientVer"`
	MaxTimeDiff  string `json:"MaxTimeDiff"`
}

type ShadowsocksNode struct {
	CommonNode
	Cipher    string `json:"cipher"`
	ServerKey string `json:"server_key"`
}

type TrojanNode struct {
	CommonNode
	Network         string          `json:"network"`
	NetworkSettings json.RawMessage `json:"networkSettings"`
}

type HysteriaNode struct {
	CommonNode
	UpMbps   int    `json:"up_mbps"`
	DownMbps int    `json:"down_mbps"`
	Obfs     string `json:"obfs"`
}

type Hysteria2Node struct {
	CommonNode
	UpMbps       int    `json:"up_mbps"`
	DownMbps     int    `json:"down_mbps"`
	ObfsType     string `json:"obfs"`
	ObfsPassword string `json:"obfs-password"`
}

type StringList []string

func (s *StringList) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*s = values
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
		values = values[:0]
		for _, line := range lines {
			if line != "" {
				values = append(values, line)
			}
		}
		*s = values
		return nil
	}

	return fmt.Errorf("expected string or string array, got %s", string(data))
}

type AnyTlsNode struct {
	CommonNode
	PaddingScheme StringList `json:"padding_scheme,omitempty"`
}

// NaiveNode is Xboard's "naive" node. Xboard sends tls as 0/1, but NaiveProxy
// clients always speak HTTPS, so V2bX serves it with TLS regardless.
type NaiveNode struct {
	CommonNode
	Tls int `json:"tls"`
}

type RawDNS struct {
	DNSMap  map[string]map[string]interface{}
	DNSJson []byte
}

type Rules struct {
	Regexp   []string
	Protocol []string
}

func (c *Client) GetNodeInfo() (node *NodeInfo, err error) {
	const path = "/api/v1/server/UniProxy/config"
	r, err := c.client.
		R().
		SetHeader("If-None-Match", c.nodeEtag).
		ForceContentType("application/json").
		Get(path)

	if err = c.checkResponse(r, path, err); err != nil {
		return nil, err
	}
	if r.StatusCode() == 304 {
		if c.responseBodyHash == "" {
			return nil, fmt.Errorf("node config returned 304 before a successful response")
		}
		return nil, nil
	}
	hash := sha256.Sum256(r.Body())
	newBodyHash := hex.EncodeToString(hash[:])
	if c.responseBodyHash == newBodyHash {
		return nil, nil
	}

	if r != nil {
		defer func() {
			if r.RawBody() != nil {
				r.RawBody().Close()
			}
		}()
	} else {
		return nil, fmt.Errorf("received nil response")
	}
	node = &NodeInfo{
		Id:   c.NodeId,
		Type: c.NodeType,
		RawDNS: RawDNS{
			DNSMap:  make(map[string]map[string]interface{}),
			DNSJson: []byte(""),
		},
	}
	// parse protocol params
	var cm *CommonNode
	switch c.NodeType {
	case "vmess", "vless":
		rsp := &VAllssNode{}
		err = json.Unmarshal(r.Body(), rsp)
		if err != nil {
			return nil, fmt.Errorf("decode v2ray params error: %s", err)
		}
		if len(rsp.NetworkSettingsBack) > 0 {
			rsp.NetworkSettings = rsp.NetworkSettingsBack
			rsp.NetworkSettingsBack = nil
		}
		if rsp.TlsSettingsBack != nil {
			rsp.TlsSettings = *rsp.TlsSettingsBack
			rsp.TlsSettingsBack = nil
		}
		cm = &rsp.CommonNode
		node.VAllss = rsp
		node.Security = node.VAllss.Tls
		if len(rsp.NetworkSettings) > 0 {
			err = json.Unmarshal(rsp.NetworkSettings, &rsp.RealityConfig)
			if err != nil {
				return nil, fmt.Errorf("decode reality config error: %s", err)
			}
		}
		if node.Security == Reality {
			if rsp.TlsSettings.PrivateKey == "" {
				key := crypt.GenX25519Private([]byte("vless" + c.Token))
				rsp.TlsSettings.PrivateKey = base64.RawURLEncoding.EncodeToString(key)
			}
		}
	case "shadowsocks":
		rsp := &ShadowsocksNode{}
		err = json.Unmarshal(r.Body(), rsp)
		if err != nil {
			return nil, fmt.Errorf("decode shadowsocks params error: %s", err)
		}
		cm = &rsp.CommonNode
		node.Shadowsocks = rsp
		node.Security = None
	case "trojan":
		rsp := &TrojanNode{}
		err = json.Unmarshal(r.Body(), rsp)
		if err != nil {
			return nil, fmt.Errorf("decode trojan params error: %s", err)
		}
		cm = &rsp.CommonNode
		node.Trojan = rsp
		node.Security = Tls
	case "hysteria":
		rsp := &HysteriaNode{}
		err = json.Unmarshal(r.Body(), rsp)
		if err != nil {
			return nil, fmt.Errorf("decode hysteria params error: %s", err)
		}
		cm = &rsp.CommonNode
		node.Hysteria = rsp
		node.Security = Tls
	case "hysteria2":
		rsp := &Hysteria2Node{}
		err = json.Unmarshal(r.Body(), rsp)
		if err != nil {
			return nil, fmt.Errorf("decode hysteria2 params error: %s", err)
		}
		cm = &rsp.CommonNode
		node.Hysteria2 = rsp
		node.Security = Tls
	case "anytls":
		rsp := &AnyTlsNode{}
		err = json.Unmarshal(r.Body(), rsp)
		if err != nil {
			return nil, fmt.Errorf("decode anytls params error: %s", err)
		}
		cm = &rsp.CommonNode
		node.AnyTls = rsp
		node.Security = Tls
	case "naive":
		rsp := &NaiveNode{}
		err = json.Unmarshal(r.Body(), rsp)
		if err != nil {
			return nil, fmt.Errorf("decode naive params error: %s", err)
		}
		cm = &rsp.CommonNode
		node.Naive = rsp
		node.Security = Tls
	}

	// parse rules and dns
	if cm == nil || cm.ServerPort < 1 || cm.ServerPort > 65535 {
		return nil, fmt.Errorf("node config has no valid server_port")
	}
	for i := range cm.Routes {
		var matchs []string
		switch match := cm.Routes[i].Match.(type) {
		case string:
			matchs = strings.Split(match, ",")
		case []string:
			matchs = match
		case []interface{}:
			for _, value := range match {
				text, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("route match must contain strings")
				}
				matchs = append(matchs, text)
			}
		default:
			return nil, fmt.Errorf("route match must be a string or string list")
		}
		if len(matchs) == 0 {
			return nil, fmt.Errorf("route match is empty")
		}
		switch cm.Routes[i].Action {
		case "block":
			for _, v := range matchs {
				if strings.HasPrefix(v, "protocol:") {
					// protocol
					node.Rules.Protocol = append(node.Rules.Protocol, strings.TrimPrefix(v, "protocol:"))
				} else {
					// domain
					node.Rules.Regexp = append(node.Rules.Regexp, strings.TrimPrefix(v, "regexp:"))
				}
			}
		case "dns":
			var domains []string
			domains = append(domains, matchs...)
			if matchs[0] != "main" {
				node.RawDNS.DNSMap[strconv.Itoa(i)] = map[string]interface{}{
					"address": cm.Routes[i].ActionValue,
					"domains": domains,
				}
			} else {
				dns := []byte(strings.Join(matchs[1:], ""))
				node.RawDNS.DNSJson = dns
			}
		}
	}

	// set interval
	node.PushInterval, node.PullInterval = time.Minute, time.Minute
	if cm.BaseConfig != nil {
		node.PushInterval = intervalToTime(cm.BaseConfig.PushInterval)
		node.PullInterval = intervalToTime(cm.BaseConfig.PullInterval)
	}

	node.Common = cm
	// clear
	cm.Routes = nil
	cm.BaseConfig = nil
	c.responseBodyHash = newBodyHash
	c.nodeEtag = r.Header().Get("ETag")

	return node, nil
}

func intervalToTime(i interface{}) time.Duration {
	var seconds float64
	switch value := i.(type) {
	case int:
		seconds = float64(value)
	case float64:
		seconds = value
	case string:
		seconds, _ = strconv.ParseFloat(value, 64)
	}
	if math.IsNaN(seconds) || seconds < 1 || seconds > 365*24*60*60 {
		return time.Minute
	}
	return time.Duration(seconds * float64(time.Second))
}
