package starter

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

func (r *Runner) stopAll(parent context.Context) error {
	stopCtx, cancel := context.WithTimeout(parent, defaultStopTimeout)
	defer cancel()

	stopTargets := r.totalStopTargets()
	errCh := make(chan error, stopTargets)
	var wg sync.WaitGroup

	for _, rw := range r.workers {
		for thread := range rw.stoppers {
			worker := rw.stoppers[thread]
			wg.Add(1)
			go func(w Worker, thread int) {
				defer wg.Done()
				if r.hooks.OnWorkerStop != nil {
					r.hooks.OnWorkerStop(workerName(w), thread)
				}
				if err := w.Stop(stopCtx); err != nil && !errors.Is(err, context.Canceled) {
					wrapped := fmt.Errorf("runner: worker %q stop failed: %w", workerName(w), err)
					if r.hooks.OnWorkerError != nil {
						r.hooks.OnWorkerError(workerName(w), thread, WorkerStageStop, wrapped)
					}
					errCh <- wrapped
				}
			}(worker, thread)
		}
	}

	wg.Wait()
	close(errCh)

	var joined error
	for err := range errCh {
		joined = errors.Join(joined, err)
	}

	return joined
}
