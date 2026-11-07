package task

import (
	"testing"
	"time"
)

func TestCallbackCanChangeIntervalAndCloseItself(t *testing.T) {
	done := make(chan struct{})
	var task Task
	task = Task{Interval: time.Millisecond, Execute: func() error { task.SetInterval(time.Second); task.Close(); close(done); return nil }}
	go task.Start(true)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback deadlocked on its task")
	}
	task.Wait()
}

func TestCloseAndWaitDrainCallback(t *testing.T) {
	entered, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	task := &Task{Interval: time.Millisecond, Execute: func() error { close(entered); <-release; return nil }}
	if err := task.Start(false); err != nil {
		t.Fatal(err)
	}
	<-entered
	go func() { task.Close(); task.Wait(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Wait returned before callback finished")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Wait did not drain callback")
	}
}
