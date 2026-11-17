package limiter

import (
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/format"
	"github.com/InazumaV/V2bX/conf"
)

func TestDeviceLimitKeepsIPsFromPreviousReport(t *testing.T) {
	const tag = "device-limit"
	user := panel.UserInfo{Id: 1, Uuid: "u1", DeviceLimit: 2}
	l := AddLimiter(tag, &conf.LimitConfig{}, []panel.UserInfo{user}, map[int]int{})
	t.Cleanup(func() { DeleteLimiter(tag) })
	key := format.UserTag(tag, user.Uuid)

	for _, ip := range []string{"10.0.0.1", "10.0.0.2"} {
		if _, reject := l.CheckLimit(key, ip, true, true); reject {
			t.Fatalf("%s rejected before limit", ip)
		}
	}
	if _, err := l.GetOnlineDevice(); err != nil {
		t.Fatal(err)
	}
	// The panel now counts both reported IPs as alive.
	l.UpdateAlive(map[int]int{user.Id: 2})

	if _, reject := l.CheckLimit(key, "10.0.0.1", true, true); reject {
		t.Fatal("first reported IP rejected")
	}
	// Regression: the second already-counted IP used to be rejected once
	// another IP had reconnected first in the new report window.
	if _, reject := l.CheckLimit(key, "10.0.0.2", true, true); reject {
		t.Fatal("second reported IP rejected")
	}
	if _, reject := l.CheckLimit(key, "10.0.0.3", true, true); !reject {
		t.Fatal("new IP over the device limit accepted")
	}
}

func TestSpeedLimitChangeReplacesBucket(t *testing.T) {
	const tag = "speed-limit"
	user := panel.UserInfo{Id: 1, Uuid: "u1", SpeedLimit: 8}
	l := AddLimiter(tag, &conf.LimitConfig{}, []panel.UserInfo{user}, map[int]int{})
	t.Cleanup(func() { DeleteLimiter(tag) })
	key := format.UserTag(tag, user.Uuid)

	before, _ := l.CheckLimit(key, "10.0.0.1", true, true)
	updated := user
	updated.SpeedLimit = 16
	l.UpdateUser(tag, []panel.UserInfo{updated}, []panel.UserInfo{user})
	after, _ := l.CheckLimit(key, "10.0.0.1", true, true)
	if before == nil || after == nil || after.Rate() != 2*before.Rate() {
		t.Fatalf("bucket rate not updated: %v -> %v", before, after)
	}
}

func TestDeletedUserIsRejected(t *testing.T) {
	const tag = "deleted-user"
	user := panel.UserInfo{Id: 1, Uuid: "u1"}
	l := AddLimiter(tag, &conf.LimitConfig{}, []panel.UserInfo{user}, map[int]int{})
	t.Cleanup(func() { DeleteLimiter(tag) })
	l.UpdateUser(tag, nil, []panel.UserInfo{user})
	if _, reject := l.CheckLimit(format.UserTag(tag, user.Uuid), "10.0.0.1", true, true); !reject {
		t.Fatal("deleted user accepted")
	}
}
