package node

import (
	"github.com/InazumaV/V2bX/api/panel"
	log "github.com/sirupsen/logrus"
)

func (c *Controller) reportUserTrafficTask() (err error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.reportUserTraffic()
}

// Caller holds stateMu; pending usage survives a failed panel response.
func (c *Controller) reportUserTraffic() (err error) {
	if c.pendingTraffic == nil {
		c.pendingTraffic = make(map[int]panel.UserTraffic)
	}
	// Get User traffic
	userTraffic := make([]panel.UserTraffic, 0)
	for i := range c.userList {
		up, down := c.server.GetUserTraffic(c.tag, c.userList[i].Uuid, true)
		if up > 0 || down > 0 {
			if c.LimitConfig.EnableDynamicSpeedLimit {
				c.traffic[c.userList[i].Uuid] += up + down
			}
			uid := c.userList[i].Id
			pending := c.pendingTraffic[uid]
			pending.UID = uid
			pending.Upload += up
			pending.Download += down
			c.pendingTraffic[uid] = pending
		}
	}
	for _, traffic := range c.pendingTraffic {
		userTraffic = append(userTraffic, traffic)
	}
	if len(userTraffic) > 0 {
		err = c.apiClient.ReportUserTraffic(userTraffic)
		if err != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			}).Info("Report user traffic failed")
		} else {
			clear(c.pendingTraffic)
			log.WithField("tag", c.tag).Infof("Report %d users traffic", len(userTraffic))
		}
	}

	if onlineDevice, err := c.limiter.GetOnlineDevice(); err != nil {
		log.Print(err)
	} else if len(*onlineDevice) > 0 {
		// Only report user has traffic > 100kb to allow ping test
		var result []panel.OnlineUser
		var nocountUID = make(map[int]struct{})
		for _, traffic := range userTraffic {
			total := traffic.Upload + traffic.Download
			if total < int64(c.Options.DeviceOnlineMinTraffic*1000) {
				nocountUID[traffic.UID] = struct{}{}
			}
		}
		for _, online := range *onlineDevice {
			if _, ok := nocountUID[online.UID]; !ok {
				result = append(result, online)
			}
		}
		data := make(map[int][]string)
		for _, onlineuser := range result {
			// json structure: { UID1:["ip1","ip2"],UID2:["ip3","ip4"] }
			data[onlineuser.UID] = append(data[onlineuser.UID], onlineuser.IP)
		}
		if err = c.apiClient.ReportNodeOnlineUsers(&data); err != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			}).Info("Report online users failed")
		} else {
			log.WithField("tag", c.tag).Infof("Total %d online users, %d Reported", len(*onlineDevice), len(result))
		}
	}

	userTraffic = nil
	return nil
}

func compareUserList(old, new []panel.UserInfo) (deleted, added []panel.UserInfo) {
	type identity struct {
		uuid               string
		id, speed, devices int
	}
	key := func(u panel.UserInfo) identity { return identity{u.Uuid, u.Id, u.SpeedLimit, u.DeviceLimit} }
	oldMap := make(map[identity]int)
	for i, user := range old {
		oldMap[key(user)] = i
	}

	for _, user := range new {
		if _, exists := oldMap[key(user)]; !exists {
			added = append(added, user)
		} else {
			delete(oldMap, key(user))
		}
	}

	for _, index := range oldMap {
		deleted = append(deleted, old[index])
	}

	return deleted, added
}

// coreUserChanges keeps only users whose core identity (UUID + panel ID)
// really changed. A user that appears in both lists only had its speed or
// device limit changed; the limiter handles that, so the core does not need
// to rebuild its inbound and drop every connection on the node.
func coreUserChanges(deleted, added []panel.UserInfo) (coreDeleted, coreAdded []panel.UserInfo) {
	type identity struct {
		uuid string
		id   int
	}
	inDeleted := make(map[identity]bool, len(deleted))
	for _, u := range deleted {
		inDeleted[identity{u.Uuid, u.Id}] = true
	}
	inAdded := make(map[identity]bool, len(added))
	for _, u := range added {
		inAdded[identity{u.Uuid, u.Id}] = true
	}
	for _, u := range deleted {
		if !inAdded[identity{u.Uuid, u.Id}] {
			coreDeleted = append(coreDeleted, u)
		}
	}
	for _, u := range added {
		if !inDeleted[identity{u.Uuid, u.Id}] {
			coreAdded = append(coreAdded, u)
		}
	}
	return coreDeleted, coreAdded
}
