package panel

import (
	"fmt"
	"io"
	"strings"

	"github.com/goccy/go-json"
	"github.com/vmihailenco/msgpack/v5"
)

type OnlineUser struct {
	UID int
	IP  string
}

type UserInfo struct {
	Id          int    `json:"id" msgpack:"id"`
	Uuid        string `json:"uuid" msgpack:"uuid"`
	SpeedLimit  int    `json:"speed_limit" msgpack:"speed_limit"`
	DeviceLimit int    `json:"device_limit" msgpack:"device_limit"`
}

type UserListBody struct {
	Users []UserInfo `json:"users" msgpack:"users"`
}

type AliveMap struct {
	Alive map[int]int `json:"alive"`
}

// GetUserList will pull user from v2board
func (c *Client) GetUserList() ([]UserInfo, error) {
	const path = "/api/v1/server/UniProxy/user"
	r, err := c.client.R().
		SetHeader("If-None-Match", c.userEtag).
		SetHeader("X-Response-Format", "msgpack").
		SetDoNotParseResponse(true).
		Get(path)
	if r != nil && r.RawResponse != nil {
		defer r.RawResponse.Body.Close()
	}
	if err = c.checkResponse(r, path, err); err != nil {
		return nil, err
	}
	if r.RawResponse == nil {
		return nil, fmt.Errorf("panel request %s returned no response", path)
	}
	if r.StatusCode() == 304 {
		return nil, nil
	}

	userList := &UserListBody{}
	if strings.Contains(r.Header().Get("Content-Type"), "application/x-msgpack") {
		if err := msgpack.NewDecoder(r.RawResponse.Body).Decode(userList); err != nil {
			return nil, fmt.Errorf("decode user list error: %w", err)
		}
	} else {
		body, err := io.ReadAll(r.RawResponse.Body)
		if err != nil {
			return nil, fmt.Errorf("read user list error: %w", err)
		}
		if err := json.Unmarshal(body, userList); err != nil {
			return nil, fmt.Errorf("unmarshal userlist error: %s", err)
		}
	}
	if userList.Users == nil {
		return nil, fmt.Errorf("user response must contain a users array")
	}
	c.userEtag = r.Header().Get("ETag")
	return userList.Users, nil
}

// GetUserAlive fetches the panel-side online IP count for each user.
func (c *Client) GetUserAlive() (map[int]int, error) {
	const path = "/api/v1/server/UniProxy/alivelist"
	r, err := c.client.R().
		ForceContentType("application/json").
		Get(path)
	alive := &AliveMap{Alive: make(map[int]int)}
	if err != nil || r == nil || r.StatusCode() >= 399 {
		return alive.Alive, nil
	}
	if err := json.Unmarshal(r.Body(), alive); err != nil {
		return make(map[int]int), nil
	}
	if alive.Alive == nil {
		alive.Alive = make(map[int]int)
	}
	return alive.Alive, nil
}

type UserTraffic struct {
	UID      int
	Upload   int64
	Download int64
}

// ReportUserTraffic reports the user traffic
func (c *Client) ReportUserTraffic(userTraffic []UserTraffic) error {
	data := make(map[int][]int64, len(userTraffic))
	for i := range userTraffic {
		data[userTraffic[i].UID] = []int64{userTraffic[i].Upload, userTraffic[i].Download}
	}
	const path = "/api/v1/server/UniProxy/push"
	r, err := c.client.R().
		SetBody(data).
		ForceContentType("application/json").
		Post(path)
	err = c.checkResponse(r, path, err)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) ReportNodeOnlineUsers(data *map[int][]string) error {
	const path = "/api/v1/server/UniProxy/alive"
	r, err := c.client.R().
		SetBody(data).
		ForceContentType("application/json").
		Post(path)
	err = c.checkResponse(r, path, err)

	if err != nil {
		return err
	}

	return nil
}
