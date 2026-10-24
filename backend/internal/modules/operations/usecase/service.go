// Package usecase is the platform operations view (TEC-930,
// docs/OPERATIONS.md): the health of the background work and the failed
// work of every organization, for the platform operator.
//
//   - Summary: queue sizes per Asynq queue, running workers, failed
//     integration jobs, failed customer notice sends, outbox and engine job
//     failures, error tracking status.
//   - Queue tasks: the Asynq tasks that failed (retry = waiting for the next
//     automatic attempt, archived = out of attempts). Run again or delete.
//   - Integration jobs: provider errors of every organization; the retry is
//     the runner's own (CheckStatus first, resubmit only work the provider
//     confirms was not done).
//   - Notice failures: failed customer notice sends (channel, provider,
//     error). Read only: a channel is tried at most once per notice, so a
//     failed send is never sent again from here.
//
// Lists name the organization, never customer data (no recipient, no text).
package usecase

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
)

// Queue task states shown on the failed task list.
const (
	StateArchived = "archived"
	StateRetry    = "retry"
)

// MaxTasksPerQueue bounds how many failed tasks of one queue and state are
// read for one list (Asynq keeps archived tasks for 90 days, at most 10 000).
const MaxTasksPerQueue = 1000

// FailedIntegrationStatuses are the integration job statuses that need a
// person (the default filter of the provider error list).
var FailedIntegrationStatuses = []string{"failed", "needs_verification", "blocked"}

// Errors.
var (
	ErrQueueUnavailable = errors.New("operations: queue inspector unavailable")
	ErrTaskNotFound     = errors.New("operations: queue task not found")
	ErrTaskNotFailed    = errors.New("operations: queue task is not failed (only retry and archived tasks)")
	ErrInvalidState     = errors.New("operations: state must be archived or retry")
)

// Querier is the subset of db.Querier the service reads.
type Querier interface {
	ListPlatformIntegrationJobs(ctx context.Context, arg db.ListPlatformIntegrationJobsParams) ([]db.ListPlatformIntegrationJobsRow, error)
	GetPlatformIntegrationJobScope(ctx context.Context, id uuid.UUID) (int64, error)
	CountPlatformIntegrationJobsByStatus(ctx context.Context, statuses []string) ([]db.CountPlatformIntegrationJobsByStatusRow, error)
	ListPlatformNoticeFailures(ctx context.Context, arg db.ListPlatformNoticeFailuresParams) ([]db.ListPlatformNoticeFailuresRow, error)
	CountPlatformNoticeFailuresSince(ctx context.Context, since pgtype.Timestamptz) (int64, error)
	CountPlatformEngineFailuresSince(ctx context.Context, since pgtype.Timestamptz) (db.CountPlatformEngineFailuresSinceRow, error)
	CountOutboxByStatus(ctx context.Context, status string) (int64, error)
}

// QueueInspector is the part of *asynq.Inspector the service uses.
type QueueInspector interface {
	Queues() ([]string, error)
	GetQueueInfo(queue string) (*asynq.QueueInfo, error)
	Servers() ([]*asynq.ServerInfo, error)
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
	ListArchivedTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	ListRetryTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	RunTask(queue, id string) error
	DeleteTask(queue, id string) error
}

// JobRunner is the part of the integration job runner the service uses.
type JobRunner interface {
	Detail(ctx context.Context, orgID int64, id uuid.UUID) (jobsusecase.JobDetail, error)
	Retry(ctx context.Context, orgID int64, id uuid.UUID) (jobsusecase.RetryResult, error)
}

// Tracking describes the error tracking of this process.
type Tracking struct {
	Enabled     func() bool
	Environment string
}

// Service serves the operations view.
type Service struct {
	q        Querier
	insp     QueueInspector
	runner   JobRunner
	tracking Tracking
	now      func() time.Time
}

// New builds the service. insp nil = no Redis (queue parts report
// unavailable); runner nil = integration jobs cannot be opened or retried.
func New(q Querier, insp QueueInspector, runner JobRunner, tracking Tracking) *Service {
	return &Service{q: q, insp: insp, runner: runner, tracking: tracking, now: time.Now}
}

// QueueStats is one Asynq queue.
type QueueStats struct {
	Name           string  `json:"name"`
	Pending        int     `json:"pending"`
	Active         int     `json:"active"`
	Scheduled      int     `json:"scheduled"`
	Retry          int     `json:"retry"`
	Archived       int     `json:"archived"`
	ProcessedToday int     `json:"processed_today"`
	FailedToday    int     `json:"failed_today"`
	Paused         bool    `json:"paused"`
	LatencyMS      float64 `json:"latency_ms"`
}

// QueueSummary is the queue part of the summary.
type QueueSummary struct {
	Available bool         `json:"available"`
	Message   string       `json:"message,omitempty"`
	Workers   int          `json:"workers"`
	Retry     int          `json:"retry"`
	Archived  int          `json:"archived"`
	Queues    []QueueStats `json:"queues"`
}

// IntegrationSummary counts integration jobs that need a person.
type IntegrationSummary struct {
	Failed            int64 `json:"failed"`
	NeedsVerification int64 `json:"needs_verification"`
	Blocked           int64 `json:"blocked"`
}

// NoticeSummary counts failed customer notice sends.
type NoticeSummary struct {
	Failed24h int64 `json:"failed_24h"`
	Failed7d  int64 `json:"failed_7d"`
}

// OutboxSummary counts outbox events not yet published.
type OutboxSummary struct {
	Pending int64 `json:"pending"`
	Failed  int64 `json:"failed"`
}

// EngineSummary counts failed export / import / bulk jobs of the last 7 days.
type EngineSummary struct {
	Exports int64 `json:"exports"`
	Imports int64 `json:"imports"`
	Bulk    int64 `json:"bulk"`
}

// TrackingSummary is the error tracking status of the API process.
type TrackingSummary struct {
	Enabled     bool   `json:"enabled"`
	Environment string `json:"environment"`
}

// Summary is GET /v1/platform/operations/summary.
type Summary struct {
	Status        string             `json:"status"` // healthy | attention
	GeneratedAt   time.Time          `json:"generated_at"`
	Queue         QueueSummary       `json:"queue"`
	Integrations  IntegrationSummary `json:"integrations"`
	Notices       NoticeSummary      `json:"notices"`
	Outbox        OutboxSummary      `json:"outbox"`
	Engine        EngineSummary      `json:"engine"`
	ErrorTracking TrackingSummary    `json:"error_tracking"`
}

// Summary reads every count. A queue that cannot be inspected reports
// available = false instead of failing the whole summary.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	now := s.now().UTC()
	out := Summary{Status: "healthy", GeneratedAt: now, Queue: s.queueSummary()}

	counts, err := s.q.CountPlatformIntegrationJobsByStatus(ctx, FailedIntegrationStatuses)
	if err != nil {
		return Summary{}, err
	}
	for _, c := range counts {
		switch c.Status {
		case "failed":
			out.Integrations.Failed = c.Total
		case "needs_verification":
			out.Integrations.NeedsVerification = c.Total
		case "blocked":
			out.Integrations.Blocked = c.Total
		}
	}
	if out.Notices.Failed24h, err = s.q.CountPlatformNoticeFailuresSince(ctx, ts(now.Add(-24*time.Hour))); err != nil {
		return Summary{}, err
	}
	if out.Notices.Failed7d, err = s.q.CountPlatformNoticeFailuresSince(ctx, ts(now.Add(-7*24*time.Hour))); err != nil {
		return Summary{}, err
	}
	if out.Outbox.Pending, err = s.q.CountOutboxByStatus(ctx, "pending"); err != nil {
		return Summary{}, err
	}
	if out.Outbox.Failed, err = s.q.CountOutboxByStatus(ctx, "failed"); err != nil {
		return Summary{}, err
	}
	engine, err := s.q.CountPlatformEngineFailuresSince(ctx, ts(now.Add(-7*24*time.Hour)))
	if err != nil {
		return Summary{}, err
	}
	out.Engine = EngineSummary{Exports: engine.Exports, Imports: engine.Imports, Bulk: engine.Bulk}
	if s.tracking.Enabled != nil {
		out.ErrorTracking.Enabled = s.tracking.Enabled()
	}
	out.ErrorTracking.Environment = s.tracking.Environment

	if !out.Queue.Available || out.Queue.Workers == 0 || out.Queue.Archived > 0 ||
		out.Integrations.Failed+out.Integrations.NeedsVerification+out.Integrations.Blocked > 0 ||
		out.Notices.Failed24h > 0 || out.Outbox.Failed > 0 {
		out.Status = "attention"
	}
	return out, nil
}

func (s *Service) queueSummary() QueueSummary {
	out := QueueSummary{Queues: []QueueStats{}}
	if s.insp == nil {
		out.Message = "queue inspector is not configured"
		return out
	}
	names, err := s.insp.Queues()
	if err != nil {
		out.Message = err.Error()
		return out
	}
	out.Available = true
	sort.Strings(names)
	for _, name := range names {
		info, err := s.insp.GetQueueInfo(name)
		if err != nil {
			// A queue drained and removed between the two calls.
			continue
		}
		out.Queues = append(out.Queues, QueueStats{
			Name: info.Queue, Pending: info.Pending, Active: info.Active, Scheduled: info.Scheduled,
			Retry: info.Retry, Archived: info.Archived, ProcessedToday: info.Processed, FailedToday: info.Failed,
			Paused: info.Paused, LatencyMS: float64(info.Latency.Milliseconds()),
		})
		out.Retry += info.Retry
		out.Archived += info.Archived
	}
	if servers, err := s.insp.Servers(); err == nil {
		out.Workers = len(servers)
	}
	return out
}

// QueueTask is one failed Asynq task. The payload is not shown (it carries
// internal ids only, and the list is not the place to read it).
type QueueTask struct {
	ID            string     `json:"id"`
	Queue         string     `json:"queue"`
	Type          string     `json:"type"`
	State         string     `json:"state"`
	Retried       int        `json:"retried"`
	MaxRetry      int        `json:"max_retry"`
	LastError     string     `json:"last_error"`
	LastFailedAt  *time.Time `json:"last_failed_at"`
	NextProcessAt *time.Time `json:"next_process_at"`
}

// QueueTaskFilter filters the failed task list.
type QueueTaskFilter struct {
	State  string
	Queues []string
	Types  []string
	Limit  int32
	Offset int32
}

// QueueTasks lists failed tasks of every (or the named) queue, the most
// recent failure first.
func (s *Service) QueueTasks(_ context.Context, f QueueTaskFilter) ([]QueueTask, int64, error) {
	if s.insp == nil {
		return nil, 0, ErrQueueUnavailable
	}
	state := f.State
	if state == "" {
		state = StateArchived
	}
	if state != StateArchived && state != StateRetry {
		return nil, 0, ErrInvalidState
	}
	names, err := s.insp.Queues()
	if err != nil {
		return nil, 0, errors.Join(ErrQueueUnavailable, err)
	}
	wanted := set(f.Queues)
	types := set(f.Types)
	all := []QueueTask{}
	for _, name := range names {
		if len(wanted) > 0 && !wanted[name] {
			continue
		}
		var tasks []*asynq.TaskInfo
		if state == StateRetry {
			tasks, err = s.insp.ListRetryTasks(name, asynq.PageSize(MaxTasksPerQueue))
		} else {
			tasks, err = s.insp.ListArchivedTasks(name, asynq.PageSize(MaxTasksPerQueue))
		}
		if err != nil {
			if errors.Is(err, asynq.ErrQueueNotFound) {
				continue
			}
			return nil, 0, err
		}
		for _, t := range tasks {
			if len(types) > 0 && !types[t.Type] {
				continue
			}
			all = append(all, taskOf(t, state))
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i].LastFailedAt, all[j].LastFailedAt
		switch {
		case a == nil && b == nil:
			return all[i].ID < all[j].ID
		case a == nil:
			return false
		case b == nil:
			return true
		}
		return a.After(*b)
	})
	total := int64(len(all))
	start, end := window(len(all), f.Limit, f.Offset)
	return all[start:end], total, nil
}

// RunQueueTask runs a failed task again now (archived or retry only).
func (s *Service) RunQueueTask(_ context.Context, queue, id string) (QueueTask, error) {
	t, err := s.failedTask(queue, id)
	if err != nil {
		return QueueTask{}, err
	}
	if err := s.insp.RunTask(queue, id); err != nil {
		return QueueTask{}, taskErr(err)
	}
	return t, nil
}

// DeleteQueueTask drops a failed task (archived or retry only).
func (s *Service) DeleteQueueTask(_ context.Context, queue, id string) (QueueTask, error) {
	t, err := s.failedTask(queue, id)
	if err != nil {
		return QueueTask{}, err
	}
	if err := s.insp.DeleteTask(queue, id); err != nil {
		return QueueTask{}, taskErr(err)
	}
	return t, nil
}

func (s *Service) failedTask(queue, id string) (QueueTask, error) {
	if s.insp == nil {
		return QueueTask{}, ErrQueueUnavailable
	}
	queue, id = strings.TrimSpace(queue), strings.TrimSpace(id)
	if queue == "" || id == "" {
		return QueueTask{}, ErrTaskNotFound
	}
	info, err := s.insp.GetTaskInfo(queue, id)
	if err != nil {
		return QueueTask{}, taskErr(err)
	}
	switch info.State {
	case asynq.TaskStateArchived:
		return taskOf(info, StateArchived), nil
	case asynq.TaskStateRetry:
		return taskOf(info, StateRetry), nil
	default:
		return QueueTask{}, ErrTaskNotFailed
	}
}

func taskErr(err error) error {
	if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
		return ErrTaskNotFound
	}
	// An invalid queue name is a task that does not exist either.
	if strings.Contains(err.Error(), "queue name") {
		return ErrTaskNotFound
	}
	return err
}

func taskOf(t *asynq.TaskInfo, state string) QueueTask {
	return QueueTask{
		ID: t.ID, Queue: t.Queue, Type: t.Type, State: state, Retried: t.Retried, MaxRetry: t.MaxRetry,
		LastError: t.LastErr, LastFailedAt: timeOrNil(t.LastFailedAt), NextProcessAt: timeOrNil(t.NextProcessAt),
	}
}

// Organization names the business a row belongs to (nil = platform).
type Organization struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// IntegrationJob is one row of the provider error list.
type IntegrationJob struct {
	UUID          uuid.UUID     `json:"uuid"`
	Organization  *Organization `json:"organization"`
	Direction     string        `json:"direction"`
	Provider      string        `json:"provider"`
	Kind          string        `json:"kind"`
	Status        string        `json:"status"`
	Priority      string        `json:"priority"`
	Attempts      int32         `json:"attempts"`
	MaxAttempts   int32         `json:"max_attempts"`
	LastError     string        `json:"last_error"`
	RefType       string        `json:"ref_type"`
	Retryable     bool          `json:"retryable"`
	NextAttemptAt *time.Time    `json:"next_attempt_at"`
	CompletedAt   *time.Time    `json:"completed_at"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// IntegrationJobFilter filters the provider error list. No statuses = the
// ones that need a person (FailedIntegrationStatuses).
type IntegrationJobFilter struct {
	Statuses  []string
	Providers []string
	Kinds     []string
	Q         string
	SortField string
	SortDesc  bool
	Limit     int32
	Offset    int32
}

// IntegrationJobs lists the integration jobs of every organization.
func (s *Service) IntegrationJobs(ctx context.Context, f IntegrationJobFilter) ([]IntegrationJob, int64, error) {
	p := db.ListPlatformIntegrationJobsParams{
		Statuses: f.Statuses, Q: strings.TrimSpace(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if len(p.Statuses) == 0 {
		p.Statuses = FailedIntegrationStatuses
	}
	if len(f.Providers) > 0 {
		p.Providers = f.Providers
	}
	if len(f.Kinds) > 0 {
		p.Kinds = f.Kinds
	}
	if p.SortField == "" {
		p.SortField, p.SortDesc = "updated_at", true
	}
	rows, err := s.q.ListPlatformIntegrationJobs(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]IntegrationJob, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		j := IntegrationJob{
			UUID: r.Uuid, Direction: r.Direction, Provider: r.Provider, Kind: r.Kind, Status: r.Status,
			Priority: r.Priority, Attempts: r.Attempts, MaxAttempts: r.MaxAttempts, LastError: r.LastError,
			RefType: r.RefType, Retryable: jobsusecase.CanRetry(r.Kind, r.Direction, r.Status),
			NextAttemptAt: pgTime(r.NextAttemptAt), CompletedAt: pgTime(r.CompletedAt),
			CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if r.OrganizationUuid.Valid {
			j.Organization = &Organization{UUID: uuid.UUID(r.OrganizationUuid.Bytes), Name: r.OrganizationName}
		}
		out = append(out, j)
	}
	return out, total, nil
}

// IntegrationJob opens one job with its provider calls.
func (s *Service) IntegrationJob(ctx context.Context, id uuid.UUID) (jobsusecase.JobDetail, error) {
	orgID, err := s.jobScope(ctx, id)
	if err != nil {
		return jobsusecase.JobDetail{}, err
	}
	return s.runner.Detail(ctx, orgID, id)
}

// RetryIntegrationJob is the runner's retry in the job's own scope.
func (s *Service) RetryIntegrationJob(ctx context.Context, id uuid.UUID) (jobsusecase.RetryResult, error) {
	orgID, err := s.jobScope(ctx, id)
	if err != nil {
		return jobsusecase.RetryResult{}, err
	}
	return s.runner.Retry(ctx, orgID, id)
}

func (s *Service) jobScope(ctx context.Context, id uuid.UUID) (int64, error) {
	if s.runner == nil {
		return 0, jobsusecase.ErrNotFound
	}
	orgID, err := s.q.GetPlatformIntegrationJobScope(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, jobsusecase.ErrNotFound
	}
	return orgID, err
}

// NoticeFailure is one failed customer notice send.
type NoticeFailure struct {
	UUID         uuid.UUID    `json:"uuid"`
	Organization Organization `json:"organization"`
	EventCode    string       `json:"event_code"`
	Channel      string       `json:"channel"`
	Provider     string       `json:"provider"`
	AttemptNo    int32        `json:"attempt_no"`
	Error        string       `json:"error"`
	CreatedAt    time.Time    `json:"created_at"`
	FinishedAt   *time.Time   `json:"finished_at"`
}

// NoticeFailureFilter filters the failed send list.
type NoticeFailureFilter struct {
	Channels []string
	Q        string
	SortDesc bool
	Limit    int32
	Offset   int32
}

// NoticeFailures lists failed customer notice sends of every organization.
func (s *Service) NoticeFailures(ctx context.Context, f NoticeFailureFilter) ([]NoticeFailure, int64, error) {
	p := db.ListPlatformNoticeFailuresParams{
		Q: strings.TrimSpace(f.Q), SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if len(f.Channels) > 0 {
		p.Channels = f.Channels
	}
	rows, err := s.q.ListPlatformNoticeFailures(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]NoticeFailure, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, NoticeFailure{
			UUID: r.Uuid, Organization: Organization{UUID: r.OrganizationUuid, Name: r.OrganizationName},
			EventCode: r.EventCode, Channel: r.Channel, Provider: r.Provider, AttemptNo: r.AttemptNo,
			Error: r.Error, CreatedAt: r.CreatedAt.Time.UTC(), FinishedAt: pgTime(r.FinishedAt),
		})
	}
	return out, total, nil
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func pgTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	v := t.UTC()
	return &v
}

func set(values []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out[v] = true
		}
	}
	return out
}

func window(n int, limit, offset int32) (int, int) {
	start := int(offset)
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	end := n
	if limit > 0 && start+int(limit) < n {
		end = start + int(limit)
	}
	return start, end
}
