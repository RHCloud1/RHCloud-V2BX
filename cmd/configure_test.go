package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func TestConfigureIsolatesNodesAndEscapesSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	password := `dummy-"password\with~symbols`
	// hysteria2 core node 2 (self cert), then sing AnyTLS node 7 behind a SOCKS exit.
	answers := []string{"y", "https://panel.example", "dummy-api-key", "y", "3", "2", "3", "hy.example", "", "2", "7", "7", "3", "any.example", "", "y", "socks.example", "10001", "dummy-user", password, "n"}
	var output bytes.Buffer
	if err := configure(strings.NewReader(strings.Join(answers, "\n")+"\n"), &output, path); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), password) {
		t.Fatal("secret appeared in prompts")
	}
	c := conf.New()
	if err := c.LoadFromPath(path); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := checkCompiledCores(c); err != nil {
		t.Fatal(err)
	}
	if len(c.CoresConfig) != 2 || len(c.NodeConfig) != 2 {
		t.Fatal("missing cores/nodes")
	}
	if c.NodeConfig[0].Options.CoreName == c.NodeConfig[1].Options.CoreName {
		t.Fatal("nodes share a core unexpectedly")
	}
	if c.NodeConfig[0].Options.ListenIP != "" {
		t.Fatal("hysteria2 core should keep its default listener")
	}
	if c.NodeConfig[0].Options.CertConfig.CertFile == c.NodeConfig[1].Options.CertConfig.CertFile {
		t.Fatal("different domains share certificate path")
	}
	socks, err := readSocksOutbounds(c.CoresConfig[1].SingConfig.OriginalPath)
	if err != nil || len(socks) != 1 || socks[0].Password != password {
		t.Fatalf("SOCKS credentials did not round trip: %v", err)
	}
	for _, name := range []string{path, c.CoresConfig[1].SingConfig.OriginalPath} {
		stat, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if stat.Mode().Perm() != 0600 {
			t.Fatalf("unsafe permissions: %s %v", name, stat.Mode())
		}
	}
}

func TestConfigureNaiveNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// sing core, protocol 8, HTTP cert, explicit IPv6 listener, direct exit, enable HTTP/3.
	answers := []string{"", "https://panel.example", "dummy-api-key", "n", "2", "9", "8", "1", "naive.example", "::", "n", "y", "n"}
	var output bytes.Buffer
	if err := configure(strings.NewReader(strings.Join(answers, "\n")+"\n"), &output, path); err != nil {
		t.Fatal(err)
	}
	c := conf.New()
	if err := c.LoadFromPath(path); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	n := c.NodeConfig[0]
	if n.ApiConfig.NodeType != "naive" || n.Options.Core != "sing" || n.Options.CertConfig.CertMode != "http" {
		t.Fatalf("unexpected naive node: %+v", n)
	}
	if n.Options.SingOptions == nil || !n.Options.SingOptions.NaiveQUIC {
		t.Fatal("NaiveQUIC answer was not saved")
	}
	if n.Options.ListenIP != "::" {
		t.Fatal("IPv6 listener lost")
	}
	if n.Options.CertConfig.Email != defaultACMEEmail {
		t.Fatalf("ACME email should default to %s, got %q", defaultACMEEmail, n.Options.CertConfig.Email)
	}
	// Xray does not offer naive: the choice is asked again until input ends.
	output.Reset()
	answers = []string{"y", "https://panel.example", "dummy-api-key", "n", "1", "9", "8"}
	if err := configure(strings.NewReader(strings.Join(answers, "\n")+"\n"), &output, filepath.Join(t.TempDir(), "x.json")); err == nil {
		t.Fatal("expected xray naive to be rejected")
	}
	if strings.Contains(output.String(), "8. Naive") || !strings.Contains(output.String(), "无效的选择") {
		t.Fatalf("xray menu should not list naive:\n%s", output.String())
	}
}

func TestConfigureSecondPanelAndReality(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// Not fixed: the second node asks for its own panel. xray VLESS Reality needs no certificate.
	answers := []string{"y", "https://a.example", "key-a", "n", "1", "3", "2", "y", "",
		"", "https://b.example", "key-b", "2", "4", "1", "n", "", "n", "no"}
	var output bytes.Buffer
	if err := configure(strings.NewReader(strings.Join(answers, "\n")+"\n"), &output, path); err != nil {
		t.Fatalf("%v\n%s", err, output.String())
	}
	c := conf.New()
	if err := c.LoadFromPath(path); err != nil {
		t.Fatal(err)
	}
	if len(c.NodeConfig) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(c.NodeConfig))
	}
	a, b := c.NodeConfig[0], c.NodeConfig[1]
	if a.ApiConfig.APIHost != "https://a.example" || b.ApiConfig.APIHost != "https://b.example" || b.ApiConfig.Key != "key-b" {
		t.Fatal("second node did not use its own panel")
	}
	if a.ApiConfig.NodeType != "vless" || a.Options.CertConfig.CertMode != "none" || a.Options.ListenIP != "0.0.0.0" {
		t.Fatalf("unexpected reality node: %+v", a.Options)
	}
	if b.ApiConfig.NodeType != "shadowsocks" || b.ApiConfig.NodeID != 4 {
		t.Fatalf("unexpected second node: %+v", b.ApiConfig)
	}
}

func TestConfigureCancelAndInvalidInputPreserveConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte("existing-config")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"n\n", "y\nhttps://panel.example\ndummy-key\n2\ninvalid-id\n"} {
		_ = configure(strings.NewReader(input), &bytes.Buffer{}, path)
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, original) {
			t.Fatal("existing config changed on canceled/invalid wizard")
		}
	}
}

func TestWriteConfigBundleBacksUpBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	helper, main := filepath.Join(dir, "helper.json"), filepath.Join(dir, "config.json")
	if err := os.WriteFile(helper, []byte("old-helper"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(main, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigBundle(map[string][]byte{helper: []byte("new-helper"), main: []byte("new-main")}, main); err == nil {
		t.Fatal("expected directory preflight error")
	}
	data, _ := os.ReadFile(helper)
	if string(data) != "old-helper" {
		t.Fatal("helper changed before all snapshots were read")
	}
	if err := os.Remove(main); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigBundle(map[string][]byte{helper: []byte("new-helper"), main: []byte(`{"valid":true}`)}, main); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(helper + ".bak.*")
	if len(backups) == 0 {
		t.Fatal("missing backup")
	}
	data, _ = os.ReadFile(backups[0])
	if string(data) != "old-helper" {
		t.Fatal("incorrect backup")
	}
	var decoded map[string]any
	data, _ = os.ReadFile(main)
	if json.Unmarshal(data, &decoded) != nil {
		t.Fatal("invalid main file")
	}
}

func TestWriteConfigBundleRestoresEarlierWrites(t *testing.T) {
	dir := t.TempDir()
	helper, main := filepath.Join(dir, "a-helper.json"), filepath.Join(dir, "z-main.json")
	parent := filepath.Join(dir, "b-parent")
	for _, path := range []string{helper, main, parent} {
		if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	err := writeConfigBundle(map[string][]byte{helper: []byte("new-helper"), filepath.Join(parent, "child.json"): []byte("unwritable"), main: []byte("new-main")}, main)
	if err == nil {
		t.Fatal("expected write failure")
	}
	for _, path := range []string{helper, main} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "old" {
			t.Fatalf("did not restore %s: %v", path, err)
		}
	}
}
