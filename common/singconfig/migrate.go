package singconfig

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Migrate only rewrites supported forms; ambiguous configurations require manual review.
func Migrate(data []byte) ([]byte, []string, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON; convert comments/JSON5 to JSON before migration")
	}
	if root == nil {
		return nil, nil, fmt.Errorf("configuration must be a JSON object")
	}
	var changes []string
	dns, _ := root["dns"].(map[string]any)
	if dns == nil {
		dns = map[string]any{}
	}
	servers, _ := dns["servers"].([]any)
	for _, raw := range servers {
		s, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("invalid DNS server")
		}
		address, legacy := s["address"].(string)
		if !legacy {
			continue
		}
		if _, ok := s["type"]; ok {
			return nil, nil, fmt.Errorf("DNS server has both legacy address and type")
		}
		if _, ok := s["strategy"]; ok {
			return nil, nil, fmt.Errorf("per-server DNS strategy requires manual migration")
		}
		delete(s, "address")
		if address == "local" {
			s["type"] = "local"
		} else {
			if !strings.Contains(address, "://") {
				address = "udp://" + address
			}
			u, err := url.Parse(address)
			if err != nil || u.Hostname() == "" {
				return nil, nil, fmt.Errorf("unsupported DNS address")
			}
			switch u.Scheme {
			case "udp", "tcp", "tls", "quic", "https", "h3":
			default:
				return nil, nil, fmt.Errorf("unsupported DNS transport %q; migrate it manually", u.Scheme)
			}
			if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return nil, nil, fmt.Errorf("DNS address with credentials/query/fragment requires manual migration")
			}
			kind := u.Scheme
			if kind == "h3" {
				kind = "http3"
			}
			s["type"] = kind
			s["server"] = u.Hostname()
			if port := u.Port(); port != "" {
				number, err := strconv.Atoi(port)
				if err != nil || number < 1 || number > 65535 {
					return nil, nil, fmt.Errorf("invalid DNS port")
				}
				s["server_port"] = number
			}
			if u.Path != "" && u.Path != "/" {
				if kind != "https" && kind != "http3" {
					return nil, nil, fmt.Errorf("unexpected path in DNS address")
				}
				s["path"] = u.Path
			}
		}
		if resolver, ok := s["address_resolver"]; ok {
			s["domain_resolver"] = resolver
			delete(s, "address_resolver")
		}
		if _, ok := s["address_strategy"]; ok {
			return nil, nil, fmt.Errorf("DNS address_strategy requires manual migration")
		}
		changes = append(changes, "DNS address -> typed DNS server")
	}
	blocked := map[string]bool{}
	var kept []any
	outbounds, _ := root["outbounds"].([]any)
	for _, raw := range outbounds {
		o, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("invalid outbound")
		}
		if o["type"] == "block" {
			tag, _ := o["tag"].(string)
			if tag == "" {
				return nil, nil, fmt.Errorf("untagged block outbound requires manual migration")
			}
			blocked[tag] = true
			changes = append(changes, "block outbound -> reject route action")
			continue
		}
		kept = append(kept, o)
	}
	route, _ := root["route"].(map[string]any)
	if route != nil {
		if final, _ := route["final"].(string); blocked[final] {
			return nil, nil, fmt.Errorf("block used as route.final requires manual migration")
		}
		var rewrite func([]any) error
		rewrite = func(rules []any) error {
			for _, raw := range rules {
				r, ok := raw.(map[string]any)
				if !ok {
					return fmt.Errorf("invalid route rule")
				}
				if nested, ok := r["rules"].([]any); ok {
					if err := rewrite(nested); err != nil {
						return err
					}
				}
				if tag, ok := r["outbound"].(string); ok {
					if blocked[tag] {
						if action, _ := r["action"].(string); action != "" && action != "route" {
							return fmt.Errorf("block rule has incompatible action")
						}
						delete(r, "outbound")
						r["action"] = "reject"
					} else if _, ok := r["action"]; !ok {
						r["action"] = "route"
						changes = append(changes, "explicit route action")
					}
				}
			}
			return nil
		}
		if rules, ok := route["rules"].([]any); ok {
			if err := rewrite(rules); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, raw := range kept {
		o := raw.(map[string]any)
		if tag, _ := o["detour"].(string); blocked[tag] {
			return nil, nil, fmt.Errorf("detour references a removed block outbound; manual migration required")
		}
		if list, ok := o["outbounds"].([]any); ok {
			for _, tag := range list {
				if s, ok := tag.(string); ok && blocked[s] {
					return nil, nil, fmt.Errorf("selector references a removed block outbound; manual migration required")
				}
			}
		}
		if strategy, ok := o["domain_strategy"].(string); ok {
			if _, ok := o["domain_resolver"]; ok {
				return nil, nil, fmt.Errorf("both domain_strategy and domain_resolver are set")
			}
			resolver, _ := dns["final"].(string)
			if resolver == "" && len(servers) > 0 {
				server := servers[0].(map[string]any)
				resolver, _ = server["tag"].(string)
				if resolver == "" {
					resolver = "v2bx-default"
					server["tag"] = resolver
				}
			}
			if resolver == "" {
				resolver = "v2bx-local"
				servers = append(servers, map[string]any{"type": "local", "tag": resolver})
			}
			o["domain_resolver"] = map[string]any{"server": resolver, "strategy": strategy}
			delete(o, "domain_strategy")
			changes = append(changes, "outbound domain_strategy -> domain_resolver")
		}
	}
	if len(outbounds) > 0 {
		root["outbounds"] = kept
	}
	if len(servers) > 0 {
		dns["servers"] = servers
		root["dns"] = dns
	}
	// Check raw IPs in migrated DNS servers early, including bracketed IPv6 input.
	for _, raw := range servers {
		s := raw.(map[string]any)
		if host, ok := s["server"].(string); ok && strings.Contains(host, ":") && net.ParseIP(host) == nil {
			return nil, nil, fmt.Errorf("invalid DNS server address")
		}
	}
	result, err := json.MarshalIndent(root, "", "  ")
	return result, changes, err
}
