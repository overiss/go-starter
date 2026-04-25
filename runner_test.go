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

type cloneWorker struct {
	starts *atomic.Int32
	stops  *atomic.Int32
}

func (w *cloneWorker) Start(ctx context.Context) error {
	w.starts.Add(1)
	<-ctx.Done()
	return nil
}

func (w *cloneWorker) Stop(context.Context) error {
	w.stops.Add(1)
	return nil
}

func (w *cloneWorker) CloneWorker() Worker {
	return &cloneWorker{
		starts: w.starts,
		stops:  w.stops,
	}
}

func TestInitWithConfigUsesCloneableWorker(t *testing.T) {
	starts := &atomic.Int32{}
	stops := &atomic.Int32{}

	base := &cloneWorker{starts: starts, stops: stops}
	r := InitWithConfig(WorkerConfig{
		Worker:  base,
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
	if stops.Load() != 3 {
		t.Fatalf("expected 3 stop calls for cloned workers, got %d", stops.Load())
	}
}

func TestInitWithConfigUsesThreadFactory(t *testing.T) {
	var starts atomic.Int32
	var stops atomic.Int32

	r := InitWithConfig(WorkerConfig{
		Threads: 4,
		ThreadFactory: func(thread int) Worker {
			return &testWorker{
				name: "factory-worker",
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
		},
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

	if starts.Load() != 4 {
		t.Fatalf("expected 4 starts, got %d", starts.Load())
	}
	if stops.Load() != 4 {
		t.Fatalf("expected 4 stops, got %d", stops.Load())
	}
}

func TestRunContextReturnsReason(t *testing.T) {
	w := &testWorker{
		startFn: func(context.Context) error {
			return errors.New("start failed")
		},
	}
	r := Init(w)
	result := r.RunContext(context.Background())

	if result.Reason != StopReasonWorkerError {
		t.Fatalf("expected reason %q, got %q", StopReasonWorkerError, result.Reason)
	}
	if result.Err() == nil {
		t.Fatal("expected run result error")
	}
}

func TestHooksAreCalled(t *testing.T) {
	var runnerStarted atomic.Int32
	var runnerStopped atomic.Int32
	var workerStarted atomic.Int32
	var workerStopped atomic.Int32
	var workerErrored atomic.Int32

	w := &testWorker{
		startFn: func(context.Context) error {
			return errors.New("hook test")
		},
		stopFn: func(context.Context) error {
			return nil
		},
	}

	r := Init(w).Configure(WithHooks(Hooks{
		OnRunnerStart: func(context.Context) { runnerStarted.Add(1) },
		OnRunnerStop:  func(context.Context, RunResult) { runnerStopped.Add(1) },
		OnWorkerStart: func(string, int) { workerStarted.Add(1) },
		OnWorkerStop:  func(string, int) { workerStopped.Add(1) },
		OnWorkerError: func(string, int, WorkerStage, error) { workerErrored.Add(1) },
	}))

	_ = r.Run(context.Background())

	if runnerStarted.Load() != 1 || runnerStopped.Load() != 1 {
		t.Fatalf("expected runner hooks once, got start=%d stop=%d", runnerStarted.Load(), runnerStopped.Load())
	}
	if workerStarted.Load() == 0 || workerStopped.Load() == 0 || workerErrored.Load() == 0 {
		t.Fatalf("expected worker hooks to be called, got start=%d stop=%d error=%d", workerStarted.Load(), workerStopped.Load(), workerErrored.Load())
	}
}

func TestCollectAllPolicyWaitsForContextCancel(t *testing.T) {
	failing := &testWorker{
		startFn: func(context.Context) error {
			return errors.New("first error")
		},
	}
	blocking := &testWorker{
		startFn: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	}

	r := Init(failing, blocking).Configure(WithErrorPolicy(ErrorPolicyCollectAll))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan RunResult, 1)
	go func() {
		done <- r.RunContext(ctx)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case result := <-done:
		if result.Err() == nil {
			t.Fatal("expected collected error")
		}
		if result.Reason != StopReasonContextCancel {
			t.Fatalf("expected reason %q, got %q", StopReasonContextCancel, result.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("run context did not finish")
	}
}
