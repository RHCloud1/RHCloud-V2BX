package sing

import (
	"errors"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/counter"
	"github.com/InazumaV/V2bX/core"
)

func (b *Sing) AddUsers(p *core.AddUsersParams) (added int, err error) {
	state, found := b.nodes[p.Tag]
	if !found {
		return 0, errors.New("the node not found")
	}
	state.users = append(state.users, p.Users...)
	if err = b.recreateInbound(p.Tag); err != nil {
		return 0, err
	}
	return len(p.Users), nil
}

func (b *Sing) GetUserTraffic(tag, uuid string, reset bool) (up int64, down int64) {
	if v, ok := b.hookServer.counter.Load(tag); ok {
		c := v.(*counter.TrafficCounter)
		up = c.GetUpCount(uuid)
		down = c.GetDownCount(uuid)
		if reset {
			c.Reset(uuid)
		}
		return
	}
	return 0, 0
}

func (b *Sing) DelUsers(users []panel.UserInfo, tag string) error {
	state, found := b.nodes[tag]
	if !found {
		return errors.New("the node not found")
	}
	deleted := make(map[string]struct{}, len(users))
	for i := range users {
		deleted[users[i].Uuid] = struct{}{}
	}
	kept := state.users[:0]
	for i := range state.users {
		if _, ok := deleted[state.users[i].Uuid]; !ok {
			kept = append(kept, state.users[i])
		}
	}
	state.users = kept
	return b.recreateInbound(tag)
}
