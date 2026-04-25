package starter

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type testWorker struct {
	name    string
	startFn func(context.Context) error
	stopFn  func(context.Context) error
}

func (w *testWorker) Name() string {
	return w.name
}

func (w *testWorker) Start(ctx context.Context) error {
	if w.startFn != nil {
		return w.startFn(ctx)
	}
	<-ctx.Done()
	return nil
}

func (w *testWorker) Stop(ctx context.Context) error {
	if w.stopFn != nil {
		return w.stopFn(ctx)
	}
	return nil
}

func TestBuildFailsWithoutWorkers(t *testing.T) {
	r := Init()
	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected run to fail without workers")
	}
}

func TestRunStopsWorkersOnContextCancel(t *testing.T) {
	var stopCalls atomic.Int32

	w := &testWorker{
		name: "main-worker",
		stopFn: func(context.Context) error {
			stopCalls.Add(1)
			return nil
		},
	}

	r := Init(w)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on context cancel, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not return after cancel")
	}

	if stopCalls.Load() != 1 {
		t.Fatalf("expected stop to be called once, got %d", stopCalls.Load())
	}
}

func TestRunReturnsStartErrorAndStopsOthers(t *testing.T) {
	var stopCalls atomic.Int32
	expectedErr := errors.New("boom")

	bad := &testWorker{
		name: "bad-worker",
		startFn: func(context.Context) error {
			return expectedErr
		},
		stopFn: func(context.Context) error {
			stopCalls.Add(1)
			return nil
		},
	}

	good := &testWorker{
		name: "good-worker",
		startFn: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		stopFn: func(context.Context) error {
			stopCalls.Add(1)
			return nil
		},
	}

	r := Init(bad, good)

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected run to return error")
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected run error to wrap start error %v, got %v", expectedErr, err)
	}

	if stopCalls.Load() != 2 {
		t.Fatalf("expected both workers to be stopped, got %d stops", stopCalls.Load())
	}
}

func TestInitWithConfigRunsWorkerInMultipleThreads(t *testing.T) {
	var starts atomic.Int32
	var stops atomic.Int32

	w := &testWorker{
		name: "threaded-worker",
		startFn: func(ctx context.Context) error {
			starts.Add(1)
			<-ctx.Done()
			return nil
		},
		stopFn: func(context.Context) error {
			stops.Add(1)
			return nil
		},
	}

	r := InitWithConfig(WorkerConfig{
		Worker:  w,
		Threads: 3,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on context cancel, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not return after cancel")
	}

	if starts.Load() != 3 {
		t.Fatalf("expected 3 start calls, got %d", starts.Load())
	}

	if stops.Load() != 1 {
		t.Fatalf("expected 1 stop call, got %d", stops.Load())
	}
}

func TestInitWithConfigUsesOneThreadByDefault(t *testing.T) {
	var starts atomic.Int32

	w := &testWorker{
		name: "default-thread-worker",
		startFn: func(ctx context.Context) error {
			starts.Add(1)
			<-ctx.Done()
			return nil
		},
	}

	r := InitWithConfig(WorkerConfig{
		Worker: w,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on context cancel, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not return after cancel")
	}

	if starts.Load() != 1 {
		t.Fatalf("expected 1 start call by default, got %d", starts.Load())
	}
}
