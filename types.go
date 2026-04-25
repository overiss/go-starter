package starter

import (
	"context"
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

// WorkerConfig describes a worker with desired parallel start threads.
type WorkerConfig struct {
	Worker  Worker
	Threads int
}

type runnerWorker struct {
	worker  Worker
	threads int
}

// Runner starts workers and gracefully stops them on cancellation/signals.
type Runner struct {
	workers      []runnerWorker
	signals      []os.Signal
	totalThreads int
}
