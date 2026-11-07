package sing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"

	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
)

var _ vCore.Core = (*Sing)(nil)

type DNSConfig struct {
	Servers []map[string]interface{} `json:"servers"`
	Rules   []map[string]interface{} `json:"rules"`
}

type Sing struct {
	nodesMu    sync.Mutex
	box        *box.Box
	ctx        context.Context
	hookServer *HookServer
	router     adapter.Router
	logFactory log.Factory
	nodes      map[string]*nodeState
}

type nodeState struct {
	inbound adapter.Inbound
	scope   *adapter.Scope
	info    *panel.NodeInfo
	config  *conf.Options
	users   []panel.UserInfo
}

func init() {
	vCore.RegisterCore("sing", New)
	vCore.RegisterConfigValidator("sing", func(c *conf.CoreConfig) error {
		if c.SingConfig == nil || c.SingConfig.OriginalPath == "" {
			return nil
		}
		_, err := readOriginal(c.SingConfig.OriginalPath)
		return err
	})
}

func singContext() context.Context {
	ctx := context.Background()
	return box.Context(ctx, include.InboundRegistry(), include.OutboundRegistry(), include.EndpointRegistry(), include.DNSTransportRegistry(), include.ServiceRegistry(), include.CertificateProviderRegistry())
}

func readOriginal(path string) (option.Options, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return option.Options{}, err
	}
	opts, err := json.UnmarshalExtendedContext[option.Options](singContext(), data)
	if err != nil {
		return opts, fmt.Errorf("invalid sing-box OriginalPath %q; inspect with V2bX migrate-sing before updating: %w", path, err)
	}
	return opts, nil
}

func New(c *conf.CoreConfig) (vCore.Core, error) {
	ctx := singContext()
	options := option.Options{}
	if len(c.SingConfig.OriginalPath) != 0 {
		var err error
		options, err = readOriginal(c.SingConfig.OriginalPath)
		if err != nil {
			return nil, fmt.Errorf("unmarshal original config error: %s", err)
		}
	}
	options.Log = &option.LogOptions{
		Disabled:  c.SingConfig.LogConfig.Disabled,
		Level:     c.SingConfig.LogConfig.Level,
		Timestamp: c.SingConfig.LogConfig.Timestamp,
		Output:    c.SingConfig.LogConfig.Output,
	}
	options.NTP = &option.NTPOptions{
		Enabled:       c.SingConfig.NtpConfig.Enable,
		WriteToSystem: true,
		ServerOptions: option.ServerOptions{
			Server:     c.SingConfig.NtpConfig.Server,
			ServerPort: c.SingConfig.NtpConfig.ServerPort,
		},
	}
	os.Setenv("SING_DNS_PATH", "")
	b, err := box.New(box.Options{
		Context: ctx,
		Options: options,
	})
	if err != nil {
		return nil, err
	}
	hs := NewHookServer()
	b.Router().AppendTracker(hs)
	return &Sing{
		ctx:        ctx,
		box:        b,
		hookServer: hs,
		router:     b.Router(),
		logFactory: b.LogFactory(),
		nodes:      make(map[string]*nodeState),
	}, nil
}

func (b *Sing) Start() error {
	return b.box.Start()
}

func (b *Sing) Close() error {
	b.nodesMu.Lock()
	defer b.nodesMu.Unlock()
	var errs []error
	for tag, state := range b.nodes {
		if state.scope != nil {
			errs = append(errs, state.scope.Close())
		}
		delete(b.nodes, tag)
	}
	return errors.Join(append(errs, b.box.Close())...)
}

func (b *Sing) Protocols() []string {
	return []string{
		"vmess",
		"vless",
		"shadowsocks",
		"trojan",
		"hysteria",
		"hysteria2",
		"anytls",
		"naive",
	}
}

func (b *Sing) Type() string {
	return "sing"
}
