package task

import (
	"log"
	"testing"
	"time"
)

func TestTask(t *testing.T) {
	ts := Task{Execute: func() error {
		log.Println("q")
		return nil
	}, Interval: time.Second}
	t.Cleanup(func() { ts.Close(); ts.Wait() })
	ts.Start(false)
}
