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
	result := r.RunContext(ctx)
	return result.Err()
}

// RunContext starts workers and returns structured execution result.
func (r *Runner) RunContext(ctx context.Context) RunResult {
	if len(r.workers) == 0 {
		return RunResult{
			Reason:     StopReasonWorkerError,
			StartError: errors.New("runner: no workers provided"),
		}
	}

	if r.hooks.OnRunnerStart != nil {
		r.hooks.OnRunnerStart(ctx)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	if len(r.signals) > 0 {
		signal.Notify(sigCh, r.signals...)
		defer signal.Stop(sigCh)
	}

	startErrCh := make(chan error, r.startErrBufferSize())
	doneCh := make(chan struct{})
	var wg sync.WaitGroup

	for _, rw := range r.workers {
		for thread := range rw.starters {
			worker := rw.starters[thread]
			wg.Add(1)
			go func(w Worker, thread int) {
				defer wg.Done()
				if r.hooks.OnWorkerStart != nil {
					r.hooks.OnWorkerStart(workerName(w), thread)
				}
				if err := w.Start(runCtx); err != nil && !errors.Is(err, context.Canceled) {
					wrapped := fmt.Errorf("runner: worker %q failed: %w", workerName(w), err)
					if r.hooks.OnWorkerError != nil {
						r.hooks.OnWorkerError(workerName(w), thread, WorkerStageStart, wrapped)
					}
					startErrCh <- wrapped
				}
			}(worker, thread)
		}
	}
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	result := RunResult{}
	startErr := r.waitShutdown(runCtx, sigCh, startErrCh, doneCh, &result)

	cancel()
	result.StopError = r.stopAll(runCtx)
	result.StartError = startErr
	if result.Reason == "" {
		result.Reason = StopReasonContextCancel
	}

	if r.hooks.OnRunnerStop != nil {
		r.hooks.OnRunnerStop(ctx, result)
	}

	return result
}

func (r *Runner) waitShutdown(
	runCtx context.Context,
	sigCh <-chan os.Signal,
	startErrCh <-chan error,
	doneCh <-chan struct{},
	result *RunResult,
) error {
	var joinedStartErr error
	for {
		select {
		case err := <-startErrCh:
			if err == nil {
				continue
			}
			joinedStartErr = errors.Join(joinedStartErr, err)
			if r.errorPolicy == ErrorPolicyFailFast {
				result.Reason = StopReasonWorkerError
				return joinedStartErr
			}
		case <-runCtx.Done():
			result.Reason = StopReasonContextCancel
			return joinedStartErr
		case sig := <-sigCh:
			result.Reason = StopReasonSignal
			result.Signal = sig
			return joinedStartErr
		case <-doneCh:
			if joinedStartErr != nil {
				result.Reason = StopReasonWorkerError
			} else {
				result.Reason = StopReasonWorkersDone
			}
			return joinedStartErr
		}
	}
}
