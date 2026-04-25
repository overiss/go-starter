package starter

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

func (r *Runner) stopAll() error {
	stopCtx, cancel := context.WithTimeout(context.Background(), defaultStopTimeout)
	defer cancel()

	errCh := make(chan error, len(r.workers))
	var wg sync.WaitGroup

	for _, rw := range r.workers {
		wg.Add(1)
		go func(w Worker) {
			defer wg.Done()
			if err := w.Stop(stopCtx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("runner: worker %q stop failed: %w", workerName(w), err)
			}
		}(rw.worker)
	}

	wg.Wait()
	close(errCh)

	var joined error
	for err := range errCh {
		joined = errors.Join(joined, err)
	}

	return joined
}
