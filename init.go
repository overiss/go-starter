package starter

import "os"

// Init creates a runner from provided workers.
func Init(workers ...Worker) *Runner {
	configs := make([]WorkerConfig, 0, len(workers))
	for _, worker := range workers {
		configs = append(configs, WorkerConfig{Worker: worker})
	}
	return InitWithConfig(configs...)
}

// Configure updates runner options in a chainable way.
func (r *Runner) Configure(opts ...RunnerOption) *Runner {
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

// InitWithConfig creates a runner from worker configs.
// If Threads is not specified or invalid, 1 thread is used.
func InitWithConfig(configs ...WorkerConfig) *Runner {
	prepared := make([]runnerWorker, 0, len(configs))
	totalThreads := 0

	for _, cfg := range configs {
		if cfg.Worker == nil && cfg.ThreadFactory == nil {
			continue
		}
		threads := cfg.Threads
		if threads <= 0 {
			threads = 1
		}

		instances, dedicatedStoppers := buildWorkerInstances(cfg, threads)
		if len(instances) == 0 {
			continue
		}

		stoppers := []Worker{cfg.Worker}
		if dedicatedStoppers {
			stoppers = instances
		}
		prepared = append(prepared, runnerWorker{starters: instances, stoppers: stoppers})
		totalThreads += len(instances)
	}

	return newRunner(prepared, totalThreads)
}

func newRunner(workers []runnerWorker, totalThreads int) *Runner {
	signalsCopy := make([]os.Signal, len(defaultSignals))
	copy(signalsCopy, defaultSignals)

	return &Runner{
		workers:      workers,
		signals:      signalsCopy,
		totalThreads: totalThreads,
		errorPolicy:  ErrorPolicyFailFast,
	}
}

func buildWorkerInstances(cfg WorkerConfig, threads int) ([]Worker, bool) {
	instances := make([]Worker, 0, threads)
	dedicatedStoppers := false
	for i := 0; i < threads; i++ {
		worker := cfg.Worker
		if cfg.ThreadFactory != nil {
			worker = cfg.ThreadFactory(i)
			dedicatedStoppers = true
		} else if threads > 1 {
			if cloneable, ok := cfg.Worker.(CloneableWorker); ok {
				worker = cloneable.CloneWorker()
				dedicatedStoppers = true
			}
		}

		if worker == nil {
			continue
		}
		instances = append(instances, worker)
	}
	return instances, dedicatedStoppers
}
