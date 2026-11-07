package panel

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/InazumaV/V2bX/conf"
	"github.com/go-resty/resty/v2"
)

// Panel is the interface for different panel's api.

type Client struct {
	client           *resty.Client
	APIHost          string
	Token            string
	NodeType         string
	NodeId           int
	nodeEtag         string
	userEtag         string
	responseBodyHash string
	LastReportOnline map[int]int
}

func New(c *conf.ApiConfig) (*Client, error) {
	client := resty.New()
	client.SetLogger(redactingLogger{token: c.Key})
	client.SetRetryCount(3)
	if c.Timeout > 0 {
		client.SetTimeout(time.Duration(c.Timeout) * time.Second)
	} else {
		client.SetTimeout(5 * time.Second)
	}
	client.SetBaseURL(c.APIHost)
	// Check node type
	c.NodeType = strings.ToLower(c.NodeType)
	switch c.NodeType {
	case "v2ray":
		c.NodeType = "vmess"
	case
		"vmess",
		"trojan",
		"shadowsocks",
		"hysteria",
		"hysteria2",
		"anytls",
		"naive",
		"vless":
	default:
		return nil, fmt.Errorf("unsupported Node type: %s", c.NodeType)
	}
	// set params
	client.SetQueryParams(map[string]string{
		"node_type": c.NodeType,
		"node_id":   strconv.Itoa(c.NodeID),
		"token":     c.Key,
	})
	return &Client{
		client:   client,
		Token:    c.Key,
		APIHost:  c.APIHost,
		NodeType: c.NodeType,
		NodeId:   c.NodeID,
	}, nil
}

type redactingLogger struct{ token string }

// Called by the serialized node controller after a rejected live update.
func (c *Client) InvalidateCache() {
	c.nodeEtag, c.userEtag, c.responseBodyHash = "", "", ""
}

func (l redactingLogger) message(format string, args ...interface{}) string {
	message := fmt.Sprintf(format, args...)
	if l.token != "" {
		message = strings.ReplaceAll(message, url.QueryEscape(l.token), "[redacted]")
		message = strings.ReplaceAll(message, l.token, "[redacted]")
	}
	return message
}
func (l redactingLogger) Errorf(format string, args ...interface{}) {
	logrus.Error(l.message(format, args...))
}
func (l redactingLogger) Warnf(format string, args ...interface{}) {
	logrus.Warn(l.message(format, args...))
}
func (l redactingLogger) Debugf(format string, args ...interface{}) {
	logrus.Debug(l.message(format, args...))
}
