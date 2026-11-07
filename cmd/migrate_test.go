package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationCommandPreviewWriteAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "original.json")
	input := []byte(`{"dns":{"servers":[{"tag":"local","address":"local"}]},"outbounds":[{"type":"direct","tag":"direct","domain_strategy":"prefer_ipv4"},{"type":"block","tag":"block"}],"route":{"rules":[{"ip_is_private":true,"outbound":"block"}],"final":"direct"}}`)
	if err := os.WriteFile(path, input, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd := newMigrateCommand()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"-c", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, input) {
		t.Fatal("preview modified input")
	}
	cmd = newMigrateCommand()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"-c", path, "--write"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if bytes.Equal(data, input) || strings.Contains(string(data), "domain_strategy") {
		t.Fatal("migration did not write")
	}
	backups, _ := filepath.Glob(path + ".bak.*")
	if len(backups) != 1 {
		t.Fatal("missing timestamp backup")
	}
	data, _ = os.ReadFile(backups[0])
	if !bytes.Equal(data, input) {
		t.Fatal("backup did not preserve old config")
	}
}
