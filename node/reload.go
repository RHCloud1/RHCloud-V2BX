package node

import (
	"errors"
	"fmt"

	"github.com/InazumaV/V2bX/api/panel"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
)

func (c *Controller) installPanelNode(info *panel.NodeInfo, users []panel.UserInfo, alive map[int]int) error {
	if info == nil {
		return fmt.Errorf("panel returned no initial node configuration")
	}
	c.tag = c.Options.Name
	if c.tag == "" {
		c.tag = c.buildNodeTag(info)
	}
	c.limiter = limiter.AddLimiter(c.tag, &c.LimitConfig, users, alive)
	if err := c.limiter.UpdateRule(&info.Rules); err != nil {
		return fmt.Errorf("update node rules: %w", err)
	}
	if info.Security == panel.Tls {
		if err := c.requestCert(); err != nil {
			return fmt.Errorf("request certificate: %w", err)
		}
	}
	if err := c.server.AddNode(c.tag, info, c.Options); err != nil {
		return fmt.Errorf("add node: %w", err)
	}
	c.nodeAdded = true
	if _, err := c.server.AddUsers(&vCore.AddUsersParams{Tag: c.tag, Users: users, NodeInfo: info}); err != nil {
		return fmt.Errorf("add users: %w", err)
	}
	c.info, c.userList, c.aliveMap = info, users, alive
	return nil
}

// Caller holds stateMu. Keep the last working panel settings for a failed rebind.
func (c *Controller) replacePanelNode(info *panel.NodeInfo, users []panel.UserInfo, alive map[int]int) error {
	oldInfo, oldUsers, oldAlive := c.info, c.userList, c.aliveMap
	if users == nil {
		users = oldUsers
	}
	_ = c.reportUserTraffic()
	if err := c.server.DelNode(c.tag); err != nil {
		c.apiClient.InvalidateCache()
		return err
	}
	c.nodeAdded = false
	limiter.DeleteLimiter(c.tag)
	if err := c.installPanelNode(info, users, alive); err != nil {
		c.apiClient.InvalidateCache()
		var cleanup error
		if c.nodeAdded {
			cleanup = c.server.DelNode(c.tag)
			c.nodeAdded = false
		}
		limiter.DeleteLimiter(c.tag)
		if restore := c.installPanelNode(oldInfo, oldUsers, oldAlive); restore != nil {
			return errors.Join(err, cleanup, fmt.Errorf("restore previous node failed: %w", restore))
		}
		return errors.Join(fmt.Errorf("new node rejected; previous node restored: %w", err), cleanup)
	}
	return nil
}
