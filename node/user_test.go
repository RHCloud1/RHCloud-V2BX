package node

import (
	"github.com/InazumaV/V2bX/api/panel"
	"testing"
)

func TestCompareUserListTracksLimitsAndIdentity(t *testing.T) {
	original := panel.UserInfo{Id: 1, Uuid: "user", SpeedLimit: 2, DeviceLimit: 3}
	for _, updated := range []panel.UserInfo{{Id: 2, Uuid: "user", SpeedLimit: 2, DeviceLimit: 3}, {Id: 1, Uuid: "user", SpeedLimit: 4, DeviceLimit: 3}, {Id: 1, Uuid: "user", SpeedLimit: 2, DeviceLimit: 5}} {
		deleted, added := compareUserList([]panel.UserInfo{original}, []panel.UserInfo{updated})
		if len(deleted) != 1 || len(added) != 1 {
			t.Fatalf("change not synchronized: %+v", updated)
		}
	}
	deleted, added := compareUserList([]panel.UserInfo{original}, []panel.UserInfo{})
	if len(deleted) != 1 || len(added) != 0 {
		t.Fatal("empty array must revoke all users")
	}
}

func TestLimitOnlyChangesSkipCore(t *testing.T) {
	old := []panel.UserInfo{{Id: 1, Uuid: "a", SpeedLimit: 2}, {Id: 2, Uuid: "b"}, {Id: 3, Uuid: "c"}}
	// a: speed limit changed; b: removed; c: panel ID changed; d: new.
	next := []panel.UserInfo{{Id: 1, Uuid: "a", SpeedLimit: 9}, {Id: 30, Uuid: "c"}, {Id: 4, Uuid: "d"}}
	deleted, added := compareUserList(old, next)
	coreDeleted, coreAdded := coreUserChanges(deleted, added)
	names := func(users []panel.UserInfo) map[string]bool {
		m := map[string]bool{}
		for _, u := range users {
			m[u.Uuid] = true
		}
		return m
	}
	if d := names(coreDeleted); len(d) != 2 || !d["b"] || !d["c"] {
		t.Fatalf("core deletions = %+v", coreDeleted)
	}
	if a := names(coreAdded); len(a) != 2 || !a["c"] || !a["d"] {
		t.Fatalf("core additions = %+v", coreAdded)
	}
	// The limiter still receives the full diff, including a's new limit.
	if !names(added)["a"] || !names(deleted)["a"] {
		t.Fatal("limiter would miss the speed limit change")
	}
}
