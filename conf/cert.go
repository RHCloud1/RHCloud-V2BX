package conf

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CertConfig struct {
	CertMode         string            `json:"CertMode"` // none, file, http, dns
	RejectUnknownSni bool              `json:"RejectUnknownSni"`
	CertDomain       string            `json:"CertDomain"`
	CertFile         string            `json:"CertFile"`
	KeyFile          string            `json:"KeyFile"`
	Provider         string            `json:"Provider"` // alidns, cloudflare, gandi, godaddy....
	Email            string            `json:"Email"`
	DNSEnv           map[string]string `json:"DNSEnv"`
}

func NewCertConfig() *CertConfig {
	return &CertConfig{
		CertMode: "none",
	}
}

func (c *CertConfig) UnmarshalJSON(data []byte) error {
	type plain CertConfig
	if err := json.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}
	if strings.Contains(c.CertFile+c.KeyFile, "{domain}") &&
		(c.CertDomain == "" || c.CertDomain == "." || c.CertDomain == ".." || strings.ContainsAny(c.CertDomain, "/\\\x00")) {
		return fmt.Errorf("CertDomain must be a domain or IP address when using {domain} in certificate paths")
	}
	r := strings.NewReplacer("{domain}", c.CertDomain, "{email}", c.Email)
	c.CertFile = r.Replace(c.CertFile)
	c.KeyFile = r.Replace(c.KeyFile)
	return nil
}
