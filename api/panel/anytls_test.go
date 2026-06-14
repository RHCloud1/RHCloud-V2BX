package panel

import (
	"reflect"
	"testing"

	"github.com/InazumaV/V2bX/conf"
	"github.com/goccy/go-json"
)

func TestAnyTlsPaddingSchemeArray(t *testing.T) {
	var node AnyTlsNode
	err := json.Unmarshal([]byte(`{"padding_scheme":["stop=8","0=30-30","1=100-400"]}`), &node)
	if err != nil {
		t.Fatal(err)
	}

	want := StringList{"stop=8", "0=30-30", "1=100-400"}
	if !reflect.DeepEqual(node.PaddingScheme, want) {
		t.Fatalf("padding scheme = %#v, want %#v", node.PaddingScheme, want)
	}
}

func TestAnyTlsPaddingSchemeString(t *testing.T) {
	var node AnyTlsNode
	err := json.Unmarshal([]byte(`{"padding_scheme":"stop=8\n0=30-30\n1=100-400"}`), &node)
	if err != nil {
		t.Fatal(err)
	}

	want := StringList{"stop=8", "0=30-30", "1=100-400"}
	if !reflect.DeepEqual(node.PaddingScheme, want) {
		t.Fatalf("padding scheme = %#v, want %#v", node.PaddingScheme, want)
	}
}

func TestNewAcceptsAnyTLS(t *testing.T) {
	client, err := New(&conf.ApiConfig{
		APIHost:  "https://panel.example.com",
		Key:      "test-key",
		NodeType: "AnyTLS",
		NodeID:   7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.NodeType != "anytls" {
		t.Fatalf("node type = %q, want anytls", client.NodeType)
	}
}
