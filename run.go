package starter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

// Run starts all workers in separate goroutines and blocks until shutdown.
func (r *Runner) Run(ctx context.Context) error {
	if len(r.workers) == 0 {
		return errors.New("runner: no workers provided")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	if len(r.signals) > 0 {
		signal.Notify(sigCh, r.signals...)
		defer signal.Stop(sigCh)
	}

	startErrCh := make(chan error, r.startErrBufferSize())
	var wg sync.WaitGroup

	for _, rw := range r.workers {
		for i := 0; i < rw.threads; i++ {
			wg.Add(1)
			go func(w Worker) {
				defer wg.Done()
				if err := w.Start(runCtx); err != nil && !errors.Is(err, context.Canceled) {
					startErrCh <- fmt.Errorf("runner: worker %q failed: %w", workerName(w), err)
				}
			}(rw.worker)
		}
	}

	var runErr error
	select {
	case runErr = <-startErrCh:
	case <-ctx.Done():
	case <-sigCh:
	}

	cancel()
	stopErr := r.stopAll()
	wg.Wait()

	return errors.Join(runErr, stopErr)
}
