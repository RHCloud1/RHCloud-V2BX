package conf

import (
	"path/filepath"
	"testing"
)

func TestConf_LoadFromPath(t *testing.T) {
	c := New()
	if err := c.LoadFromPath("../example/config.json"); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConf_Watch(t *testing.T) {
	c := New()
	if err := c.Watch(filepath.Join(t.TempDir(), "missing.json"), "", "", func() { t.Error("unexpected reload") }); err == nil {
		t.Fatal("expected a missing file error")
	}
}
