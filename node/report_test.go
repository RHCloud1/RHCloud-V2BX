package node

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
	"github.com/goccy/go-json"
)

// trafficCore hands out queued traffic once per user, like a reset read.
type trafficCore struct {
	vCore.Core
	mu      sync.Mutex
	traffic map[string][2]int64
}

func (f *trafficCore) GetUserTraffic(_, uuid string, reset bool) (int64, int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := f.traffic[uuid]
	if reset {
		delete(f.traffic, uuid)
	}
	return t[0], t[1]
}

func (f *trafficCore) add(uuid string, up, down int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := f.traffic[uuid]
	f.traffic[uuid] = [2]int64{t[0] + up, t[1] + down}
}

func TestReportMinTrafficDefersSmallUsage(t *testing.T) {
	var mu sync.Mutex
	var pushes []map[int][]int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/server/UniProxy/push" {
			body, _ := io.ReadAll(r.Body)
			data := map[int][]int64{}
			if err := json.Unmarshal(body, &data); err != nil {
				t.Error(err)
			}
			mu.Lock()
			pushes = append(pushes, data)
			mu.Unlock()
		}
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(s.Close)
	api, err := panel.New(&conf.ApiConfig{APIHost: s.URL, Key: "k", NodeType: "vless", NodeID: 1})
	if err != nil {
		t.Fatal(err)
	}
	users := []panel.UserInfo{{Id: 1, Uuid: "big"}, {Id: 2, Uuid: "small"}}
	core := &trafficCore{traffic: map[string][2]int64{}}
	c := NewController(core, api, &conf.Options{ReportMinTraffic: 10})
	c.tag, c.userList = "report-min", users
	c.limiter = limiter.AddLimiter(c.tag, &conf.LimitConfig{}, users, map[int]int{})
	t.Cleanup(func() { limiter.DeleteLimiter(c.tag) })

	core.add("big", 20*1024, 0)
	core.add("small", 4*1024, 2*1024)
	if err := c.reportUserTraffic(); err != nil {
		t.Fatal(err)
	}
	core.add("small", 3*1024, 2*1024)
	if err := c.reportUserTraffic(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(pushes) != 2 {
		t.Fatalf("pushes = %v", pushes)
	}
	if _, ok := pushes[0][2]; ok || pushes[0][1][0] != 20*1024 {
		t.Fatalf("first push = %v, want only the user above 10KB", pushes[0])
	}
	if got := pushes[1][2]; len(got) != 2 || got[0] != 7*1024 || got[1] != 4*1024 {
		t.Fatalf("second push = %v, want accumulated small usage", pushes[1])
	}
	if len(c.pendingTraffic) != 0 {
		t.Fatalf("reported traffic left pending: %v", c.pendingTraffic)
	}
}
