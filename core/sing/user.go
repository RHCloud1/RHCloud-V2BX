package sing

import (
	"errors"
	"fmt"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/counter"
	"github.com/InazumaV/V2bX/core"
)

func (b *Sing) AddUsers(p *core.AddUsersParams) (added int, err error) {
	b.nodesMu.Lock()
	defer b.nodesMu.Unlock()
	state, found := b.nodes[p.Tag]
	if !found {
		return 0, errors.New("the node not found")
	}
	users := append([]panel.UserInfo(nil), state.users...)
	seen := make(map[string]bool, len(users))
	for _, u := range users {
		seen[u.Uuid] = true
	}
	for _, u := range p.Users {
		if u.Uuid == "" {
			return 0, errors.New("user UUID/password cannot be empty")
		}
		if !seen[u.Uuid] {
			users = append(users, u)
			seen[u.Uuid] = true
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}
	if err := b.replaceUsers(p.Tag, state, users); err != nil {
		return 0, err
	}
	return added, nil
}

// Caller holds nodesMu. A failed update must not discard the last usable users.
func (b *Sing) replaceUsers(tag string, state *nodeState, users []panel.UserInfo) error {
	if len(users) == 0 {
		if state.scope != nil {
			if err := state.scope.Close(); err != nil {
				return err
			}
		}
		state.scope, state.inbound, state.users = nil, nil, users
		return nil
	}
	if _, err := getInboundOptions(tag, state.info, state.config, users); err != nil {
		return err
	}
	previous := state.users
	state.users = users
	if err := b.recreateInbound(tag); err != nil {
		state.users = previous
		if len(previous) == 0 {
			return fmt.Errorf("update inbound: %w", err)
		}
		if rollbackErr := b.recreateInbound(tag); rollbackErr != nil {
			return fmt.Errorf("update inbound: %w; restore previous inbound: %v", err, rollbackErr)
		}
		return fmt.Errorf("update inbound failed; previous users restored: %w", err)
	}
	return nil
}

func (b *Sing) GetUserTraffic(tag, uuid string, reset bool) (up int64, down int64) {
	if v, ok := b.hookServer.counter.Load(tag); ok {
		c := v.(*counter.TrafficCounter)
		if reset {
			return c.Take(uuid)
		}
		up = c.GetUpCount(uuid)
		down = c.GetDownCount(uuid)
		return
	}
	return 0, 0
}

func (b *Sing) DelUsers(users []panel.UserInfo, tag string) error {
	b.nodesMu.Lock()
	defer b.nodesMu.Unlock()
	state, found := b.nodes[tag]
	if !found {
		return errors.New("the node not found")
	}
	deleted := make(map[string]struct{}, len(users))
	for i := range users {
		deleted[users[i].Uuid] = struct{}{}
	}
	kept := make([]panel.UserInfo, 0, len(state.users))
	for i := range state.users {
		if _, ok := deleted[state.users[i].Uuid]; !ok {
			kept = append(kept, state.users[i])
		}
	}
	if len(kept) == len(state.users) {
		return nil
	}
	return b.replaceUsers(tag, state, kept)
}
