package sing

import (
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"github.com/sagernet/sing-box/option"
)

func TestGetAnyTLSInboundOptions(t *testing.T) {
	info := &panel.NodeInfo{
		Type:     "anytls",
		Security: panel.Tls,
		Common: &panel.CommonNode{
			ServerPort: 443,
		},
		AnyTls: &panel.AnyTlsNode{
			PaddingScheme: panel.StringList{"stop=8", "0=30-30"},
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
	users := []panel.UserInfo{{Uuid: "test-user-password"}}

	inbound, err := getInboundOptions("node-anytls-1", info, config, users)
	if err != nil {
		t.Fatal(err)
	}
	if inbound.Type != "anytls" {
		t.Fatalf("inbound type = %q, want anytls", inbound.Type)
	}

	options, ok := inbound.Options.(*option.AnyTLSInboundOptions)
	if !ok {
		t.Fatalf("inbound options type = %T", inbound.Options)
	}
	if len(options.Users) != 1 || options.Users[0].Password != users[0].Uuid {
		t.Fatalf("users = %#v", options.Users)
	}
	if len(options.PaddingScheme) != 2 || options.PaddingScheme[0] != "stop=8" {
		t.Fatalf("padding scheme = %#v", options.PaddingScheme)
	}
	if options.TLS == nil || !options.TLS.Enabled {
		t.Fatal("AnyTLS inbound TLS is not enabled")
	}
}

func TestGetAnyTLSInboundOptionsRequiresTLS(t *testing.T) {
	info := &panel.NodeInfo{
		Type:     "anytls",
		Security: panel.Tls,
		Common:   &panel.CommonNode{ServerPort: 443},
		AnyTls:   &panel.AnyTlsNode{},
	}
	config := &conf.Options{
		ListenIP:    "0.0.0.0",
		SingOptions: conf.NewSingOptions(),
		CertConfig:  conf.NewCertConfig(),
	}

	if _, err := getInboundOptions("node-anytls-1", info, config, nil); err == nil {
		t.Fatal("expected AnyTLS without a certificate to fail")
	}
}
