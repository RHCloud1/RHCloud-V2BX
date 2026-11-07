package singconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMigrationPreservesRoutingAndCredentials(t *testing.T) {
	input := `{"dns":{"servers":[{"tag":"dns","address":"tls://1.1.1.1:853"}]},"outbounds":[{"tag":"home","type":"socks","username":"dummy-user","password":"dummy-secret","domain_strategy":"prefer_ipv4"},{"tag":"deny","type":"block"}],"route":{"rules":[{"ip_is_private":true,"outbound":"deny"},{"domain_suffix":["example.com"],"outbound":"home"}],"final":"home"}}`
	result, changes, err := Migrate([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) < 3 {
		t.Fatalf("missing changes: %v", changes)
	}
	var config map[string]any
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	out := config["outbounds"].([]any)
	if len(out) != 1 || out[0].(map[string]any)["password"] != "dummy-secret" {
		t.Fatal("outbound credentials lost")
	}
	if strings.Contains(string(result), "domain_strategy") || strings.Contains(string(result), `"address"`) {
		t.Fatal("legacy fields retained")
	}
	rules := config["route"].(map[string]any)["rules"].([]any)
	if rules[0].(map[string]any)["action"] != "reject" || rules[0].(map[string]any)["ip_is_private"] != true {
		t.Fatal("block condition lost")
	}
	if rules[1].(map[string]any)["action"] != "route" {
		t.Fatal("missing route action")
	}
	second, changes, err := Migrate(result)
	if err != nil || len(changes) != 0 || string(second) != string(result) {
		t.Fatalf("not idempotent: %v %v", changes, err)
	}
}

func TestDNSMigration(t *testing.T) {
	for _, address := range []string{"local", "1.1.1.1", "tcp://[2606:4700:4700::1111]:53", "https://dns.example/dns-query", "h3://dns.example/dns-query"} {
		t.Run(address, func(t *testing.T) {
			input, _ := json.Marshal(map[string]any{"dns": map[string]any{"servers": []any{map[string]any{"address": address}}}})
			if _, _, err := Migrate(input); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAmbiguousMigrationRefused(t *testing.T) {
	for _, input := range []string{
		`null`, `{"dns":{"servers":[{"address":"rcode://success"}]}}`,
		`{"dns":{"servers":[{"address":"local","strategy":"ipv4_only"}]}}`,
		`{"outbounds":[{"type":"block","tag":"block"}],"route":{"final":"block"}}`,
		`{"outbounds":[{"type":"block","tag":"block"},{"type":"selector","outbounds":["block"]}]}`,
		`{"outbounds":[{"type":"block","tag":"block"},{"type":"socks","detour":"block"}]}`,
	} {
		if _, _, err := Migrate([]byte(input)); err == nil {
			t.Fatalf("accepted ambiguous input: %s", input)
		}
	}
}
