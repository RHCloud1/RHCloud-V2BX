package hy2

import (
	"sync"

	"github.com/InazumaV/V2bX/common/counter"
	"github.com/InazumaV/V2bX/common/format"
	"github.com/InazumaV/V2bX/limiter"
	"github.com/apernet/hysteria/core/v2/server"
	"go.uber.org/zap"
)

type HookServer struct {
	Tag     string
	logger  *zap.Logger
	Counter sync.Map
}

func (h *HookServer) LogTraffic(id string, tx, rx uint64) (ok bool) {
	limiterinfo, err := limiter.GetLimiter(h.Tag)
	if err != nil {
		h.logger.Error("Get limiter error", zap.String("tag", h.Tag), zap.Error(err))
		return false
	}

	if limiterinfo.ConsumeOverLimit(format.UserTag(h.Tag, id)) {
		return false
	}

	c, _ := h.Counter.LoadOrStore(h.Tag, counter.NewTrafficCounter())

	if tc, ok := c.(*counter.TrafficCounter); ok {
		tc.Rx(id, int(rx))
		tc.Tx(id, int(tx))
		return true
	}

	return false
}

func (s *HookServer) LogOnlineState(id string, online bool) {
}

func (s *HookServer) TraceStream(stream server.HyStream, stats *server.StreamStats) {
}

func (s *HookServer) UntraceStream(stream server.HyStream) {
}
