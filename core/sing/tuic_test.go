package sing

import (
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"github.com/sagernet/sing-box/option"
)

func TestGetTUICInboundOptions(t *testing.T) {
	info := &panel.NodeInfo{
		Type:     "tuic",
		Security: panel.Tls,
		Common:   &panel.CommonNode{ServerPort: 443},
		Tuic: &panel.TuicNode{
			CongestionControl: "bbr",
			ZeroRTTHandshake:  true,
		},
	}
	config := &conf.Options{
		ListenIP:    "0.0.0.0",
		SingOptions: conf.NewSingOptions(),
		CertConfig: &conf.CertConfig{
			CertMode: "file",
			CertFile: "/etc/V2bX/fullchain.cer",
			KeyFile:  "/etc/V2bX/cert.key",
		},
	}
	users := []panel.UserInfo{{Id: 1, Uuid: "3b5f1c1e-7b39-4c86-9a2b-1d2b0a6c9e11"}}

	inbound, err := getInboundOptions("node-tuic-1", info, config, users)
	if err != nil {
		t.Fatal(err)
	}
	options, ok := inbound.Options.(*option.TUICInboundOptions)
	if inbound.Type != "tuic" || !ok {
		t.Fatalf("inbound = %q %T", inbound.Type, inbound.Options)
	}
	if len(options.Users) != 1 || options.Users[0].UUID != users[0].Uuid || options.Users[0].Password != users[0].Uuid {
		t.Fatalf("users = %#v", options.Users)
	}
	if options.CongestionControl != "bbr" || !options.ZeroRTTHandshake {
		t.Fatalf("tuic options = %+v", options)
	}
	if options.TLS == nil || len(options.TLS.ALPN) != 1 || options.TLS.ALPN[0] != "h3" {
		t.Fatalf("tuic must offer h3 ALPN: %+v", options.TLS)
	}

	config.CertConfig = &conf.CertConfig{CertMode: "none"}
	if _, err := getInboundOptions("node-tuic-1", info, config, users); err == nil {
		t.Fatal("tuic without TLS accepted")
	}
}

func TestHysteria2IgnoreClientBandwidthFromPanel(t *testing.T) {
	info := &panel.NodeInfo{
		Type:      "hysteria2",
		Security:  panel.Tls,
		Common:    &panel.CommonNode{ServerPort: 443},
		Hysteria2: &panel.Hysteria2Node{Ignore_Client_Bandwidth: true},
	}
	config := &conf.Options{
		ListenIP:    "0.0.0.0",
		SingOptions: conf.NewSingOptions(),
		CertConfig: &conf.CertConfig{
			CertMode: "file",
			CertFile: "/etc/V2bX/fullchain.cer",
			KeyFile:  "/etc/V2bX/cert.key",
		},
	}
	inbound, err := getInboundOptions("node-hy2-1", info, config, []panel.UserInfo{{Uuid: "u"}})
	if err != nil {
		t.Fatal(err)
	}
	if options := inbound.Options.(*option.Hysteria2InboundOptions); !options.IgnoreClientBandwidth {
		t.Fatal("ignore_client_bandwidth from panel was dropped")
	}
}
