package starter

import "fmt"

func workerName(worker Worker) string {
	if named, ok := worker.(NamedWorker); ok {
		if name := named.Name(); name != "" {
			return name
		}
	}
	return fmt.Sprintf("%T", worker)
}

func (r *Runner) startErrBufferSize() int {
	if r.totalThreads > 0 {
		return r.totalThreads
	}
	return 1
}

func (r *Runner) totalStopTargets() int {
	total := 0
	for _, rw := range r.workers {
		total += len(rw.stoppers)
	}
	if total > 0 {
		return total
	}
	return 1
}
