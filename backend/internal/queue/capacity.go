package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Capacity holds (TEC-898, docs/design/capacity-holds.md).
const (
	// TaskCapacityExpire expires lapsed holds and frees their allocations.
	TaskCapacityExpire = "app:capacity:expire-holds"

	capacityExpireCron = "@every 1m"
)

// CapacityExpireFunc runs the sweep and returns the holds it expired.
type CapacityExpireFunc func(ctx context.Context) (int, error)

// WithCapacityExpiry registers the hold expiry sweep.
func (w *Worker) WithCapacityExpiry(expire CapacityExpireFunc) *Worker {
	w.mux.HandleFunc(TaskCapacityExpire, func(ctx context.Context, _ *asynq.Task) error {
		if expire == nil {
			return nil
		}
		n, err := expire(ctx)
		if n > 0 {
			w.log.Info("capacity_holds_expired", "holds", n)
		}
		return err
	})
	return w
}

// RegisterCapacityExpirySchedule adds the minute sweep. Lapsed holds stop
// counting at their expiry anyway; the sweep only records it and frees the
// rows.
func RegisterCapacityExpirySchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		capacityExpireCron, asynq.NewTask(TaskCapacityExpire, []byte("{}")),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(0),
		asynq.Timeout(55*time.Second), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register capacity expiry: %w", err)
	}
	return nil
}
