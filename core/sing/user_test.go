package sing

import (
	"sync"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	"github.com/InazumaV/V2bX/core"
)

func testUserInbound(t *testing.T, info *panel.NodeInfo) *Sing {
	t.Helper()
	c, err := New(&conf.CoreConfig{Type: "sing", SingConfig: conf.NewSingConfig()})
	if err != nil {
		t.Fatal(err)
	}
	b := c.(*Sing)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	if info == nil {
		info = &panel.NodeInfo{Type: "vmess", Common: &panel.CommonNode{ServerPort: 0}, VAllss: &panel.VAllssNode{Network: "tcp"}}
	}
	if err := b.AddNode("test", info, &conf.Options{ListenIP: "127.0.0.1", SingOptions: conf.NewSingOptions()}); err != nil {
		t.Fatal(err)
	}
	_, err = b.AddUsers(&core.AddUsersParams{Tag: "test", Users: []panel.UserInfo{{Uuid: "11111111-1111-4111-8111-111111111111"}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUserNoopKeepsInbound(t *testing.T) {
	b := testUserInbound(t, nil)
	before := b.nodes["test"].inbound
	if before == nil {
		t.Fatal("listener not created")
	}
	added, err := b.AddUsers(&core.AddUsersParams{Tag: "test", Users: []panel.UserInfo{{Uuid: "11111111-1111-4111-8111-111111111111"}}})
	if err != nil || added != 0 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	if err := b.DelUsers([]panel.UserInfo{{Uuid: "missing"}}, "test"); err != nil {
		t.Fatal(err)
	}
	after := b.nodes["test"].inbound
	if before != after {
		t.Fatal("no-op update recreated the listener")
	}
}

func TestFailedUserUpdateRestoresInbound(t *testing.T) {
	b := testUserInbound(t, &panel.NodeInfo{Type: "shadowsocks", Common: &panel.CommonNode{ServerPort: 0},
		Shadowsocks: &panel.ShadowsocksNode{Cipher: "2022-blake3-aes-128-gcm", ServerKey: "MDEyMzQ1Njc4OWFiY2RlZg=="}})
	_, err := b.AddUsers(&core.AddUsersParams{Tag: "test", Users: []panel.UserInfo{{Uuid: "short"}}})
	if err == nil {
		t.Fatal("expected invalid Shadowsocks-2022 password error")
	}
	if len(b.nodes["test"].users) != 1 {
		t.Fatal("failed update retained uncommitted users")
	}
	if b.nodes["test"].inbound == nil {
		t.Fatal("previous inbound was not restored")
	}
}

func TestConcurrentDuplicateUserUpdates(t *testing.T) {
	b := testUserInbound(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := b.AddUsers(&core.AddUsersParams{Tag: "test", Users: []panel.UserInfo{{Uuid: "11111111-1111-4111-8111-111111111111"}}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(b.nodes["test"].users) != 1 {
		t.Fatal("duplicate users retained")
	}
}
