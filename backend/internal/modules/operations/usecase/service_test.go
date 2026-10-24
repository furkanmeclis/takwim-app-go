package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
)

type fakeQ struct {
	statusCounts []db.CountPlatformIntegrationJobsByStatusRow
	notices      int64
	outbox       map[string]int64
	engine       db.CountPlatformEngineFailuresSinceRow
	scopes       map[uuid.UUID]int64
	jobParams    db.ListPlatformIntegrationJobsParams
}

func (f *fakeQ) ListPlatformIntegrationJobs(_ context.Context, p db.ListPlatformIntegrationJobsParams) ([]db.ListPlatformIntegrationJobsRow, error) {
	f.jobParams = p
	return []db.ListPlatformIntegrationJobsRow{{
		Uuid: uuid.New(), Direction: "outbound", Kind: "sms.send", Status: "failed", Total: 1,
		OrganizationUuid: pgtype.UUID{Bytes: uuid.New(), Valid: true}, OrganizationName: "Kuaför A",
	}, {
		Uuid: uuid.New(), Direction: "inbound", Kind: "payment.webhook", Status: "failed", Total: 2,
	}}, nil
}

func (f *fakeQ) GetPlatformIntegrationJobScope(_ context.Context, id uuid.UUID) (int64, error) {
	org, ok := f.scopes[id]
	if !ok {
		return 0, pgx.ErrNoRows
	}
	return org, nil
}

func (f *fakeQ) CountPlatformIntegrationJobsByStatus(context.Context, []string) ([]db.CountPlatformIntegrationJobsByStatusRow, error) {
	return f.statusCounts, nil
}

func (f *fakeQ) ListPlatformNoticeFailures(context.Context, db.ListPlatformNoticeFailuresParams) ([]db.ListPlatformNoticeFailuresRow, error) {
	return nil, nil
}

func (f *fakeQ) CountPlatformNoticeFailuresSince(context.Context, pgtype.Timestamptz) (int64, error) {
	return f.notices, nil
}

func (f *fakeQ) CountPlatformEngineFailuresSince(context.Context, pgtype.Timestamptz) (db.CountPlatformEngineFailuresSinceRow, error) {
	return f.engine, nil
}

func (f *fakeQ) CountOutboxByStatus(_ context.Context, status string) (int64, error) {
	return f.outbox[status], nil
}

type fakeInspector struct {
	queues   map[string]*asynq.QueueInfo
	archived map[string][]*asynq.TaskInfo
	retry    map[string][]*asynq.TaskInfo
	tasks    map[string]*asynq.TaskInfo // queue/id
	servers  int
	ran      []string
	deleted  []string
	failErr  error
}

func (f *fakeInspector) Queues() ([]string, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	out := []string{}
	for name := range f.queues {
		out = append(out, name)
	}
	return out, nil
}

func (f *fakeInspector) GetQueueInfo(queue string) (*asynq.QueueInfo, error) {
	if q, ok := f.queues[queue]; ok {
		return q, nil
	}
	return nil, asynq.ErrQueueNotFound
}

func (f *fakeInspector) Servers() ([]*asynq.ServerInfo, error) {
	return make([]*asynq.ServerInfo, f.servers), nil
}

func (f *fakeInspector) GetTaskInfo(queue, id string) (*asynq.TaskInfo, error) {
	if t, ok := f.tasks[queue+"/"+id]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("asynq: %w", asynq.ErrTaskNotFound)
}

func (f *fakeInspector) ListArchivedTasks(queue string, _ ...asynq.ListOption) ([]*asynq.TaskInfo, error) {
	return f.archived[queue], nil
}

func (f *fakeInspector) ListRetryTasks(queue string, _ ...asynq.ListOption) ([]*asynq.TaskInfo, error) {
	return f.retry[queue], nil
}

func (f *fakeInspector) RunTask(queue, id string) error {
	f.ran = append(f.ran, queue+"/"+id)
	return nil
}

func (f *fakeInspector) DeleteTask(queue, id string) error {
	f.deleted = append(f.deleted, queue+"/"+id)
	return nil
}

type fakeRunner struct {
	detailOrg, retryOrg int64
}

func (f *fakeRunner) Detail(_ context.Context, orgID int64, id uuid.UUID) (jobsusecase.JobDetail, error) {
	f.detailOrg = orgID
	return jobsusecase.JobDetail{UUID: id}, nil
}

func (f *fakeRunner) Retry(_ context.Context, orgID int64, _ uuid.UUID) (jobsusecase.RetryResult, error) {
	f.retryOrg = orgID
	return jobsusecase.RetryResult{Action: "resubmitted", Status: "pending"}, nil
}

func healthyInspector() *fakeInspector {
	return &fakeInspector{
		queues: map[string]*asynq.QueueInfo{
			"default": {Queue: "default", Pending: 2, Processed: 10},
			"exports": {Queue: "exports", Retry: 1, Latency: 1500 * time.Millisecond},
		},
		servers: 1,
	}
}

func TestSummaryHealthy(t *testing.T) {
	s := New(&fakeQ{}, healthyInspector(), &fakeRunner{}, Tracking{Enabled: func() bool { return true }, Environment: "production"})
	got, err := s.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "healthy" {
		t.Fatalf("status = %s, want healthy (%+v)", got.Status, got)
	}
	if !got.Queue.Available || got.Queue.Workers != 1 || got.Queue.Retry != 1 || len(got.Queue.Queues) != 2 {
		t.Fatalf("queue = %+v", got.Queue)
	}
	// Sorted by name; latency in milliseconds.
	if got.Queue.Queues[0].Name != "default" || got.Queue.Queues[1].LatencyMS != 1500 {
		t.Fatalf("queues = %+v", got.Queue.Queues)
	}
	if !got.ErrorTracking.Enabled || got.ErrorTracking.Environment != "production" {
		t.Fatalf("tracking = %+v", got.ErrorTracking)
	}
}

func TestSummaryAttention(t *testing.T) {
	cases := map[string]struct {
		q    *fakeQ
		insp QueueInspector
	}{
		"no inspector":        {q: &fakeQ{}, insp: nil},
		"inspector error":     {q: &fakeQ{}, insp: &fakeInspector{failErr: errors.New("redis down")}},
		"no worker":           {q: &fakeQ{}, insp: &fakeInspector{queues: map[string]*asynq.QueueInfo{}}},
		"archived task":       {q: &fakeQ{}, insp: &fakeInspector{queues: map[string]*asynq.QueueInfo{"default": {Queue: "default", Archived: 1}}, servers: 1}},
		"failed integration":  {q: &fakeQ{statusCounts: []db.CountPlatformIntegrationJobsByStatusRow{{Status: "needs_verification", Total: 3}}}, insp: healthyInspector()},
		"failed notice":       {q: &fakeQ{notices: 1}, insp: healthyInspector()},
		"failed outbox event": {q: &fakeQ{outbox: map[string]int64{"failed": 1}}, insp: healthyInspector()},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := New(tc.q, tc.insp, nil, Tracking{}).Summary(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "attention" {
				t.Fatalf("status = %s, want attention", got.Status)
			}
		})
	}
	got, _ := New(&fakeQ{statusCounts: []db.CountPlatformIntegrationJobsByStatusRow{{Status: "failed", Total: 2}, {Status: "blocked", Total: 1}}}, healthyInspector(), nil, Tracking{}).Summary(context.Background())
	if got.Integrations.Failed != 2 || got.Integrations.Blocked != 1 || got.Integrations.NeedsVerification != 0 {
		t.Fatalf("integrations = %+v", got.Integrations)
	}
}

func TestQueueTasksNewestFailureFirst(t *testing.T) {
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	insp := healthyInspector()
	insp.archived = map[string][]*asynq.TaskInfo{
		"default": {
			{ID: "a", Queue: "default", Type: "app:ping", LastErr: "boom", LastFailedAt: now.Add(-time.Hour)},
			{ID: "b", Queue: "default", Type: "app:ping", LastFailedAt: now},
		},
		"exports": {{ID: "c", Queue: "exports", Type: "app:export:process", LastFailedAt: now.Add(-time.Minute)}},
	}
	s := New(&fakeQ{}, insp, nil, Tracking{})

	got, total, err := s.QueueTasks(context.Background(), QueueTaskFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(got) != 2 || got[0].ID != "b" || got[1].ID != "c" || got[0].State != StateArchived {
		t.Fatalf("got %d %+v", total, got)
	}
	got, total, _ = s.QueueTasks(context.Background(), QueueTaskFilter{Limit: 2, Offset: 2})
	if total != 3 || len(got) != 1 || got[0].ID != "a" || got[0].LastError != "boom" {
		t.Fatalf("page 2 = %+v", got)
	}
	got, total, _ = s.QueueTasks(context.Background(), QueueTaskFilter{Queues: []string{"exports"}})
	if total != 1 || got[0].ID != "c" {
		t.Fatalf("queue filter = %+v", got)
	}
	got, total, _ = s.QueueTasks(context.Background(), QueueTaskFilter{Types: []string{"app:ping"}})
	if total != 2 {
		t.Fatalf("type filter = %+v", got)
	}
	got, total, _ = s.QueueTasks(context.Background(), QueueTaskFilter{State: StateRetry})
	if total != 0 || len(got) != 0 {
		t.Fatalf("retry = %+v", got)
	}
	if _, _, err := s.QueueTasks(context.Background(), QueueTaskFilter{State: "pending"}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("pending state err = %v", err)
	}
	if _, _, err := New(&fakeQ{}, nil, nil, Tracking{}).QueueTasks(context.Background(), QueueTaskFilter{}); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("no inspector err = %v", err)
	}
}

func TestRunAndDeleteOnlyFailedTasks(t *testing.T) {
	insp := healthyInspector()
	insp.tasks = map[string]*asynq.TaskInfo{
		"default/arch":  {ID: "arch", Queue: "default", Type: "app:ping", State: asynq.TaskStateArchived},
		"default/retry": {ID: "retry", Queue: "default", Type: "app:ping", State: asynq.TaskStateRetry},
		"default/pend":  {ID: "pend", Queue: "default", Type: "app:ping", State: asynq.TaskStatePending},
	}
	s := New(&fakeQ{}, insp, nil, Tracking{})
	ctx := context.Background()

	if got, err := s.RunQueueTask(ctx, "default", "arch"); err != nil || got.State != StateArchived {
		t.Fatalf("run archived = %+v, %v", got, err)
	}
	if _, err := s.DeleteQueueTask(ctx, "default", "retry"); err != nil {
		t.Fatalf("delete retry: %v", err)
	}
	if _, err := s.RunQueueTask(ctx, "default", "pend"); !errors.Is(err, ErrTaskNotFailed) {
		t.Fatalf("run pending err = %v", err)
	}
	if _, err := s.DeleteQueueTask(ctx, "default", "pend"); !errors.Is(err, ErrTaskNotFailed) {
		t.Fatalf("delete pending err = %v", err)
	}
	if _, err := s.RunQueueTask(ctx, "default", "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := s.RunQueueTask(ctx, "", "arch"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("empty queue err = %v", err)
	}
	if len(insp.ran) != 1 || insp.ran[0] != "default/arch" || len(insp.deleted) != 1 || insp.deleted[0] != "default/retry" {
		t.Fatalf("ran %v deleted %v", insp.ran, insp.deleted)
	}
}

func TestIntegrationJobsUseTheJobScope(t *testing.T) {
	tenantJob, platformJob := uuid.New(), uuid.New()
	q := &fakeQ{scopes: map[uuid.UUID]int64{tenantJob: 42, platformJob: 0}}
	runner := &fakeRunner{}
	s := New(q, nil, runner, Tracking{})
	ctx := context.Background()

	if _, err := s.IntegrationJob(ctx, tenantJob); err != nil || runner.detailOrg != 42 {
		t.Fatalf("detail org = %d, %v", runner.detailOrg, err)
	}
	if res, err := s.RetryIntegrationJob(ctx, tenantJob); err != nil || runner.retryOrg != 42 || res.Action != "resubmitted" {
		t.Fatalf("retry org = %d, %+v, %v", runner.retryOrg, res, err)
	}
	runner.retryOrg = -1
	if _, err := s.RetryIntegrationJob(ctx, platformJob); err != nil || runner.retryOrg != 0 {
		t.Fatalf("platform job org = %d, %v", runner.retryOrg, err)
	}
	if _, err := s.IntegrationJob(ctx, uuid.New()); !errors.Is(err, jobsusecase.ErrNotFound) {
		t.Fatalf("unknown job err = %v", err)
	}
	if _, err := New(q, nil, nil, Tracking{}).RetryIntegrationJob(ctx, tenantJob); !errors.Is(err, jobsusecase.ErrNotFound) {
		t.Fatalf("no runner err = %v", err)
	}
}

func TestIntegrationJobsDefaults(t *testing.T) {
	q := &fakeQ{}
	items, total, err := New(q, nil, nil, Tracking{}).IntegrationJobs(context.Background(), IntegrationJobFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.jobParams.Statuses) != len(FailedIntegrationStatuses) || q.jobParams.SortField != "updated_at" || !q.jobParams.SortDesc {
		t.Fatalf("params = %+v", q.jobParams)
	}
	if q.jobParams.Providers != nil || q.jobParams.Kinds != nil {
		t.Fatalf("empty filters must stay NULL: %+v", q.jobParams)
	}
	if total != 2 || items[0].Organization == nil || items[0].Organization.Name != "Kuaför A" || !items[0].Retryable {
		t.Fatalf("items = %+v", items)
	}
	// A platform job has no organization; inbound jobs are never retried.
	if items[1].Organization != nil || items[1].Retryable {
		t.Fatalf("platform job = %+v", items[1])
	}
}
