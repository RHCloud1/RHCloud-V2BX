package cmd

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestCheckMissingConfig(t *testing.T) {
	c := newCheckCommand()
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"-c", filepath.Join(t.TempDir(), "missing.json")})
	if err := c.Execute(); err == nil {
		t.Fatal("expected missing configuration error")
	}
}
