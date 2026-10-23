package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Integration job runner (TEC-880, docs/design/integration-jobs.md).
const (
	// TaskIntegrationRun runs the next step of one integration job.
	TaskIntegrationRun = "app:integrations:run"
	// TaskIntegrationSweep runs due steps (lost tasks, delivery reports).
	TaskIntegrationSweep = "app:integrations:sweep"

	// QueueIntegrations carries normal provider work.
	QueueIntegrations = "integrations"
	// QueueIntegrationsCritical carries OTP / transactional SMS: its own
	// queue with a higher weight, so a backlog of other work never delays a
	// code.
	QueueIntegrationsCritical = "integrations_critical"

	// IntegrationRunMaxRetry bounds asynq's retries of a step that failed
	// on infrastructure (DB). Provider retries are the runner's own
	// (max_attempts, exponential backoff).
	IntegrationRunMaxRetry = 5

	integrationSweepCron = "@every 1m"
)

// IntegrationRunPayload names the job (organization 0 = platform job).
type IntegrationRunPayload struct {
	OrganizationID int64 `json:"organization_id"`
	JobID          int64 `json:"job_id"`
}

// IntegrationRunFunc runs one step.
type IntegrationRunFunc func(ctx context.Context, orgID, jobID int64) error

// IntegrationSweepFunc runs the sweep and returns the steps run.
type IntegrationSweepFunc func(ctx context.Context) (int, error)

// NewIntegrationRunTask builds a step task.
func NewIntegrationRunTask(orgID, jobID int64) (*asynq.Task, error) {
	body, err := json.Marshal(IntegrationRunPayload{OrganizationID: orgID, JobID: jobID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal integration run: %w", err)
	}
	return asynq.NewTask(TaskIntegrationRun, body), nil
}

// IntegrationRetryDelay is the asynq retry delay of integration tasks:
// exponential (10s, 20s, 40s … capped at 10 minutes).
func IntegrationRetryDelay(n int) time.Duration {
	d := 10 * time.Second
	for i := 0; i < n && d < 10*time.Minute; i++ {
		d *= 2
	}
	if d > 10*time.Minute {
		d = 10 * time.Minute
	}
	return d
}

func retryDelay(n int, err error, task *asynq.Task) time.Duration {
	if task != nil && task.Type() == TaskIntegrationRun {
		return IntegrationRetryDelay(n)
	}
	return asynq.DefaultRetryDelayFunc(n, err, task)
}

// IntegrationDispatcher queues runner steps (implements the runner's
// Dispatcher).
type IntegrationDispatcher struct{ Client *Client }

// DispatchIntegrationJob enqueues one step, after delay.
func (d IntegrationDispatcher) DispatchIntegrationJob(orgID, jobID int64, critical bool, delay time.Duration) error {
	task, err := NewIntegrationRunTask(orgID, jobID)
	if err != nil {
		return err
	}
	q := QueueIntegrations
	if critical {
		q = QueueIntegrationsCritical
	}
	opts := []asynq.Option{asynq.Queue(q), asynq.MaxRetry(IntegrationRunMaxRetry), asynq.Timeout(2 * time.Minute)}
	if delay > 0 {
		opts = append(opts, asynq.ProcessIn(delay))
	}
	_, err = d.Client.Enqueue(task, opts...)
	return err
}

// WithIntegrations registers the step handler and the sweep.
func (w *Worker) WithIntegrations(run IntegrationRunFunc, sweep IntegrationSweepFunc) *Worker {
	w.mux.HandleFunc(TaskIntegrationRun, func(ctx context.Context, task *asynq.Task) error {
		var p IntegrationRunPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		if run == nil {
			return nil
		}
		return run(ctx, p.OrganizationID, p.JobID)
	})
	w.mux.HandleFunc(TaskIntegrationSweep, func(ctx context.Context, _ *asynq.Task) error {
		if sweep == nil {
			return nil
		}
		n, err := sweep(ctx)
		if n > 0 {
			w.log.Info("integration_jobs_swept", "steps", n)
		}
		return err
	})
	return w
}

// RegisterIntegrationSweepSchedule adds the minute sweep.
func RegisterIntegrationSweepSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		integrationSweepCron, asynq.NewTask(TaskIntegrationSweep, []byte("{}")),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(0),
		asynq.Timeout(55*time.Second), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register integration sweep: %w", err)
	}
	return nil
}
