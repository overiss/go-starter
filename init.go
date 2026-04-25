package starter

import "os"

// Init creates a runner from provided workers.
func Init(workers ...Worker) *Runner {
	prepared := make([]runnerWorker, 0, len(workers))
	totalThreads := 0

	for _, worker := range workers {
		if worker == nil {
			continue
		}
		prepared = append(prepared, runnerWorker{
			worker:  worker,
			threads: 1,
		})
		totalThreads++
	}

	return newRunner(prepared, totalThreads)
}

// InitWithConfig creates a runner from worker configs.
// If Threads is not specified or invalid, 1 thread is used.
func InitWithConfig(configs ...WorkerConfig) *Runner {
	prepared := make([]runnerWorker, 0, len(configs))
	totalThreads := 0

	for _, cfg := range configs {
		if cfg.Worker == nil {
			continue
		}
		threads := cfg.Threads
		if threads <= 0 {
			threads = 1
		}

		prepared = append(prepared, runnerWorker{
			worker:  cfg.Worker,
			threads: threads,
		})
		totalThreads += threads
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
	}
}
