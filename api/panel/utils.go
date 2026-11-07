package panel

import (
	"fmt"
	"github.com/go-resty/resty/v2"
)

// Debug set the client debug for client
func (c *Client) Debug() {
	c.client.SetDebug(true)
}

func (c *Client) checkResponse(res *resty.Response, path string, err error) error {
	if err != nil {
		return fmt.Errorf("panel request %s failed (transport/TLS error)", path)
	}
	if res == nil {
		return fmt.Errorf("panel request %s returned no response", path)
	}
	if res.StatusCode() >= 400 {
		return fmt.Errorf("panel request %s failed: HTTP %d", path, res.StatusCode())
	}
	return nil
}
