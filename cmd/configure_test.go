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
	answers := []string{"y", "https://panel.example", "dummy-api-key", "3", "2", "::", "3", "hy.example", "y", "2", "7", "7", "0.0.0.0", "3", "any.example", "y", "socks.example", "10001", "dummy-user", password, "n"}
	var output bytes.Buffer
	if err := configure(strings.NewReader(strings.Join(answers, "\n")+"\n"), &output, path); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), password) || strings.Contains(output.String(), "dummy-api-key") {
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
	if c.NodeConfig[0].Options.ListenIP != "::" {
		t.Fatal("IPv6 listener lost")
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
	// sing core, protocol 8, HTTP cert, direct exit, enable HTTP/3.
	answers := []string{"y", "https://panel.example", "dummy-api-key", "2", "9", "8", "0.0.0.0", "1", "naive.example", "ops@example.com", "n", "y", "n"}
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
	// Xray cannot serve naive.
	answers = []string{"y", "https://panel.example", "dummy-api-key", "1", "9", "8"}
	if err := configure(strings.NewReader(strings.Join(answers, "\n")+"\n"), &output, filepath.Join(t.TempDir(), "x.json")); err == nil {
		t.Fatal("expected xray naive to be rejected")
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
