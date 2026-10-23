package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Mobile push (Expo Push Service) task types.
const (
	// TaskPushSend sends the mobile push mirror of one notification row.
	TaskPushSend = "app:push:send"
	// TaskPushReceipts checks Expo push receipts and disables dead tokens.
	TaskPushReceipts = "app:push:receipts"

	// PushSendMaxRetry bounds retries on Expo 5xx / network errors.
	PushSendMaxRetry = 3

	pushReceiptsCron = "@every 15m"
)

// PushSendPayload identifies the notification row to push.
type PushSendPayload struct {
	NotificationID int64 `json:"notification_id"`
}

// NewPushSendTask builds a mobile push task.
func NewPushSendTask(notificationID int64) (*asynq.Task, error) {
	body, err := json.Marshal(PushSendPayload{NotificationID: notificationID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal push send: %w", err)
	}
	return asynq.NewTask(TaskPushSend, body), nil
}

// PushSendOptions are the enqueue options for a mobile push.
func PushSendOptions() []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueNotifications),
		asynq.MaxRetry(PushSendMaxRetry),
		asynq.Timeout(time.Minute),
	}
}

// ParsePushSendPayload decodes a push send payload.
func ParsePushSendPayload(data []byte) (PushSendPayload, error) {
	var p PushSendPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return PushSendPayload{}, fmt.Errorf("queue: unmarshal push send: %w", err)
	}
	return p, nil
}

// NewPushReceiptsTask builds the periodic receipt check.
func NewPushReceiptsTask() *asynq.Task {
	return asynq.NewTask(TaskPushReceipts, []byte("{}"))
}

// SendPushFunc sends the mobile push for a notification id.
type SendPushFunc func(ctx context.Context, notificationID int64) error

// ProcessPushReceiptsFunc checks due receipts and returns how many tokens it disabled.
type ProcessPushReceiptsFunc func(ctx context.Context) (int, error)

// WithMobilePush registers the Expo push sender and receipt processor.
func (w *Worker) WithMobilePush(send SendPushFunc, receipts ProcessPushReceiptsFunc) *Worker {
	w.mux.HandleFunc(TaskPushSend, func(ctx context.Context, task *asynq.Task) error {
		p, err := ParsePushSendPayload(task.Payload())
		if err != nil {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		if send == nil {
			return nil
		}
		return send(ctx, p.NotificationID)
	})
	w.mux.HandleFunc(TaskPushReceipts, func(ctx context.Context, _ *asynq.Task) error {
		if receipts == nil {
			return nil
		}
		n, err := receipts(ctx)
		if n > 0 {
			w.log.Info("push_receipts_disabled_tokens", "count", n)
		}
		return err
	})
	return w
}

// RegisterPushReceiptsSchedule adds the receipt check (every 15 minutes;
// Expo recommends waiting ~15 minutes before reading receipts).
func RegisterPushReceiptsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		pushReceiptsCron, NewPushReceiptsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(0),
		asynq.Timeout(5*time.Minute), asynq.Unique(14*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register push receipts: %w", err)
	}
	return nil
}
