package counter

import (
	"sync"
	"testing"
)

func TestConcurrentCreationAndDrainPreserveAllTraffic(t *testing.T) {
	c := NewTrafficCounter()
	var writers sync.WaitGroup
	for i := 0; i < 24; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for n := 0; n < 1000; n++ {
				c.Tx("user", 1)
				c.Rx("user", 2)
			}
		}()
	}
	done := make(chan struct{})
	go func() { writers.Wait(); close(done) }()
	var up, down int64
	for {
		u, d := c.Take("user")
		up += u
		down += d
		select {
		case <-done:
			u, d = c.Take("user")
			up += u
			down += d
			if up != 24000 || down != 48000 {
				t.Fatalf("lost traffic: %d %d", up, down)
			}
			return
		default:
		}
	}
}
