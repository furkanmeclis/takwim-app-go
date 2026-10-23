package queue

import (
	"context"
	"errors"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
)

// reportTaskFailure sends a failed task attempt to the error tracker, grouped
// per task type. Attempts that will be retried are warnings; the last attempt
// (or a SkipRetry error) is an error.
func reportTaskFailure(ctx context.Context, task *asynq.Task, err error) {
	if !observability.Enabled() || task == nil || err == nil {
		return
	}
	retried, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	queueName, _ := asynq.GetQueueName(ctx)
	final := retried >= maxRetry || errors.Is(err, asynq.SkipRetry)
	level := sentry.LevelWarning
	if final {
		level = sentry.LevelError
	}
	observability.CaptureError(ctx, err, observability.Options{
		Level:       level,
		Transaction: "task " + task.Type(),
		Fingerprint: []string{"queue-task", task.Type()},
		Tags: map[string]string{
			"task_type":     task.Type(),
			"queue":         queueName,
			"retry":         strconv.Itoa(retried),
			"max_retry":     strconv.Itoa(maxRetry),
			"final_attempt": strconv.FormatBool(final),
		},
	})
}
