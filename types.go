package starter

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

const defaultStopTimeout = 15 * time.Second

var defaultSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// Worker describes a service component managed by Runner.
type Worker interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// NamedWorker can be implemented to provide readable worker names in errors.
type NamedWorker interface {
	Name() string
}

// CloneableWorker can create independent worker instances for parallel threads.
type CloneableWorker interface {
	CloneWorker() Worker
}

// ThreadFactory creates worker instances for specific thread index.
type ThreadFactory func(thread int) Worker

// WorkerConfig describes a worker with desired parallel start threads.
type WorkerConfig struct {
	Worker        Worker
	Threads       int
	ThreadFactory ThreadFactory
}

type runnerWorker struct {
	starters []Worker
	stoppers []Worker
}

// Runner starts workers and gracefully stops them on cancellation/signals.
type Runner struct {
	workers      []runnerWorker
	signals      []os.Signal
	totalThreads int
	errorPolicy  ErrorPolicy
	hooks        Hooks
}

// ErrorPolicy controls how runner reacts to start errors.
type ErrorPolicy int

const (
	// ErrorPolicyFailFast cancels run immediately on first start error.
	ErrorPolicyFailFast ErrorPolicy = iota
	// ErrorPolicyCollectAll collects start errors and waits for external shutdown.
	ErrorPolicyCollectAll
)

// WorkerStage describes lifecycle stage used in hooks.
type WorkerStage string

const (
	WorkerStageStart WorkerStage = "start"
	WorkerStageStop  WorkerStage = "stop"
)

// StopReason tells why runner started shutdown.
type StopReason string

const (
	StopReasonSignal        StopReason = "signal"
	StopReasonContextCancel StopReason = "context_cancel"
	StopReasonWorkerError   StopReason = "worker_error"
	StopReasonWorkersDone   StopReason = "workers_done"
)

// RunResult describes full execution outcome.
type RunResult struct {
	Reason     StopReason
	Signal     os.Signal
	StartError error
	StopError  error
}

// Err returns joined start/stop error.
func (r RunResult) Err() error {
	return errors.Join(r.StartError, r.StopError)
}

// Hooks allows attaching lifecycle callbacks.
type Hooks struct {
	OnRunnerStart func(ctx context.Context)
	OnRunnerStop  func(ctx context.Context, result RunResult)
	OnWorkerStart func(name string, thread int)
	OnWorkerStop  func(name string, thread int)
	OnWorkerError func(name string, thread int, stage WorkerStage, err error)
}

// RunnerOption configures runner behavior.
type RunnerOption func(*Runner)

// WithErrorPolicy sets worker error handling policy.
func WithErrorPolicy(policy ErrorPolicy) RunnerOption {
	return func(r *Runner) {
		r.errorPolicy = policy
	}
}

// WithHooks sets lifecycle hooks.
func WithHooks(hooks Hooks) RunnerOption {
	return func(r *Runner) {
		r.hooks = hooks
	}
}
