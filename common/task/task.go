package task

import (
	"fmt"
	"sync"
	"time"
)

// Task runs callbacks serially. Close stops scheduling; Wait drains a callback.
type Task struct {
	Interval time.Duration
	Execute  func() error

	access     sync.Mutex
	execution  sync.Mutex
	timer      *time.Timer
	running    bool
	generation uint64
}

func (t *Task) run(generation uint64, execute bool) error {
	t.execution.Lock()
	defer t.execution.Unlock()
	t.access.Lock()
	active := t.running && t.generation == generation
	t.access.Unlock()
	if !active {
		return nil
	}
	var err error
	if execute {
		err = t.Execute()
	}
	t.access.Lock()
	defer t.access.Unlock()
	if !t.running || t.generation != generation {
		return err
	}
	if err != nil {
		t.running = false
		return err
	}
	t.timer = time.AfterFunc(t.Interval, func() { _ = t.run(generation, true) })
	return nil
}

func (t *Task) Start(first bool) error {
	t.access.Lock()
	if t.running {
		t.access.Unlock()
		return nil
	}
	if t.Interval <= 0 || t.Execute == nil {
		t.access.Unlock()
		return fmt.Errorf("task requires a callback and positive interval")
	}
	t.running = true
	t.generation++
	generation := t.generation
	t.access.Unlock()
	return t.run(generation, first)
}

func (t *Task) SetInterval(interval time.Duration) {
	if interval <= 0 {
		return
	}
	t.access.Lock()
	t.Interval = interval
	t.access.Unlock()
}

func (t *Task) Close() {
	t.access.Lock()
	defer t.access.Unlock()
	t.running = false
	t.generation++
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}

// Do not call Wait from Execute; use it after Close from the owning controller.
func (t *Task) Wait() {
	t.execution.Lock()
	t.execution.Unlock()
}
