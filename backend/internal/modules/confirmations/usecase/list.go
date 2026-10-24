package usecase

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// ListSortColumns are the sortable fields of the confirmation list.
var ListSortColumns = map[string]string{
	"starts_at": "starts_at", "deadline_at": "deadline_at", "number": "number", "customer": "customer", "updated_at": "updated_at",
}

// countArgs are the filters the list and its counts share.
type countArgs struct {
	branches   []uuid.UUID
	startsFrom pgtype.Timestamptz
	startsTo   pgtype.Timestamptz
	q, digits  pgtype.Text
}

func argsOf(f model.ListFilter) countArgs {
	a := countArgs{}
	if len(f.Branches) > 0 {
		a.branches = f.Branches
	}
	if f.StartsFrom != nil {
		a.startsFrom = ts(*f.StartsFrom)
	}
	if f.StartsTo != nil {
		a.startsTo = ts(*f.StartsTo)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		a.q = pgtype.Text{String: strings.TrimPrefix(q, "#"), Valid: true}
		if digits := onlyDigits(q); len(digits) >= 3 {
			a.digits = pgtype.Text{String: digits, Valid: true}
		}
	}
	return a
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// List is the confirmation list of the caller's branch scope (TEC-924):
// every booking with a confirmation task, its requests, the answer and what
// needs a person.
func (e *Engine) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Item, int64, error) {
	if _, ok := ListSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "starts_at", false
	}
	a := argsOf(f)
	now := e.clock()
	p := db.ListConfirmationTasksParams{
		OrganizationID: orgID, EventCode: Event, BranchIds: orgctx.BranchScope(ctx), Branches: a.branches,
		StartsFrom: a.startsFrom, StartsTo: a.startsTo, Q: a.q, QDigits: a.digits, NowAt: ts(now),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if len(f.States) > 0 {
		p.States = f.States
	}
	if f.Attention != nil {
		p.Attention = pgtype.Bool{Bool: *f.Attention, Valid: true}
	}
	rows, err := e.q.ListConfirmationTasks(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	counts, err := e.counts(ctx, orgID, a, now)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	for _, c := range counts {
		if !matches(c, f) {
			continue
		}
		total += c.Count
	}
	on, err := ModuleOn(ctx, e.q, orgID)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, itemOf(r, on, now))
	}
	return out, total, nil
}

func (e *Engine) counts(ctx context.Context, orgID int64, a countArgs, now time.Time) ([]db.CountConfirmationTaskStatesRow, error) {
	return e.q.CountConfirmationTaskStates(ctx, db.CountConfirmationTaskStatesParams{
		OrganizationID: orgID, EventCode: Event, BranchIds: orgctx.BranchScope(ctx), Branches: a.branches,
		StartsFrom: a.startsFrom, StartsTo: a.startsTo, Q: a.q, QDigits: a.digits, NowAt: ts(now),
	})
}

// attention mirrors the list query: the visit is ahead and a person has
// to act.
func attention(c db.CountConfirmationTaskStatesRow) bool {
	return c.Upcoming && (slices.Contains(model.AttentionStates, c.State) || c.SendFailed)
}

func matches(c db.CountConfirmationTaskStatesRow, f model.ListFilter) bool {
	if len(f.States) == 0 && c.State == model.StateDropped {
		return false
	}
	if len(f.States) > 0 && !slices.Contains(f.States, c.State) {
		return false
	}
	return f.Attention == nil || *f.Attention == attention(c)
}

func itemOf(r db.ListConfirmationTasksRow, moduleOn bool, now time.Time) model.Item {
	item := model.Item{
		UUID: r.Uuid, Number: r.Number, Status: r.Status, ApprovalStatus: r.ApprovalStatus,
		ConfirmationStatus: r.ConfirmationStatus, Version: int(r.Version),
		Branch:   model.Ref{UUID: r.BranchUuid, Name: r.BranchName},
		Customer: model.Customer{UUID: r.CustomerUuid, Name: strings.TrimSpace(r.CustomerFirstName + " " + r.CustomerLastName)},
		Services: r.Services, Staff: r.StaffNames, StartsAt: r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(),
		State: r.State, TaskStatus: r.TaskStatus, Channels: r.Channels, OnNoAnswer: r.OnNoAnswer,
		AttemptsSent: int(r.AttemptsSent), AttemptsPlanned: len(r.AttemptTimes), NextAttemptAt: timePtr(r.NextAttemptAt),
		DeadlineAt: r.DeadlineAt.Time.UTC(), Answer: textPtr(r.Answer), AnswerChannel: textPtr(r.AnswerChannel),
		AnsweredAt: timePtr(r.AnsweredAt), ChangeRequestedAt: timePtr(r.ChangeRequestedAt), ChangeNote: r.ChangeNote,
		Outcome: textPtr(r.Outcome), LastError: r.LastError, FailedSends: int(r.FailedSends), SendFailed: r.SendFailed,
		Attention: r.Attention, UpdatedAt: r.TaskUpdatedAt.Time.UTC(),
	}
	if item.Channels == nil {
		item.Channels = []string{}
	}
	if r.CustomerPhone.Valid {
		item.Customer.Phone = &r.CustomerPhone.String
	}
	if r.LastSendStatus != "" {
		item.LastSend = &model.LastSend{Status: r.LastSendStatus, Error: r.LastSendError, At: r.LastSendAt.Time.UTC()}
	}
	answerable := open(r.TaskStatus) || (r.TaskStatus == model.TaskExpired && r.Outcome.String != model.OutcomeCancelled)
	item.CanConfirm = moduleOn && answerable && r.Status == reservationsmodel.StatusActive && now.Before(r.StartsAt.Time)
	return item
}

// Facets counts the list per state with the other filters applied, and
// how many rows need a person.
func (e *Engine) Facets(ctx context.Context, orgID int64, f model.ListFilter) (model.Facets, error) {
	counts, err := e.counts(ctx, orgID, argsOf(f), e.clock())
	if err != nil {
		return model.Facets{}, err
	}
	by := map[string]int64{}
	out := model.Facets{State: []model.Facet{}}
	for _, c := range counts {
		by[c.State] += c.Count
		if attention(c) {
			out.Attention += c.Count
		}
	}
	for _, s := range model.States {
		out.State = append(out.State, model.Facet{Value: s, Count: by[s]})
	}
	return out, nil
}

// Summary is the follow-up alert of the caller's branch scope: upcoming
// visits whose confirmation needs a person (the nav badge, the alert of
// the panel).
func (e *Engine) Summary(ctx context.Context, orgID int64) (model.Summary, error) {
	counts, err := e.counts(ctx, orgID, countArgs{}, e.clock())
	if err != nil {
		return model.Summary{}, err
	}
	var out model.Summary
	for _, c := range counts {
		if !c.Upcoming {
			continue
		}
		if attention(c) {
			out.Attention += c.Count
		}
		switch c.State {
		case model.StateChangeRequested:
			out.ChangeRequested += c.Count
		case model.StateUnreachable:
			out.Unreachable += c.Count
		case model.StateFlagged:
			out.Flagged += c.Count
		case model.StateScheduled, model.StatePending:
			out.Waiting += c.Count
		}
		if c.SendFailed {
			out.SendFailed += c.Count
		}
	}
	return out, nil
}

// reservationRef finds a booking of the caller's branch scope.
func (e *Engine) reservationRef(ctx context.Context, orgID int64, id uuid.UUID) (db.GetConfirmationReservationRefRow, error) {
	r, err := e.q.GetConfirmationReservationRef(ctx, db.GetConfirmationReservationRefParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if orgctx.CheckBranch(ctx, r.BranchID) != nil {
		return r, ErrNotFound
	}
	return r, nil
}

// Attempts lists the requests of one booking's confirmation, one row per
// channel tried, newest first.
func (e *Engine) Attempts(ctx context.Context, orgID int64, id uuid.UUID) ([]model.Attempt, error) {
	r, err := e.reservationRef(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	rows, err := e.q.ListConfirmationAttempts(ctx, db.ListConfirmationAttemptsParams{OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: r.ID, Valid: true}, EventCode: Event})
	if err != nil {
		return nil, err
	}
	replies, err := e.q.ListConfirmationResponses(ctx, db.ListConfirmationResponsesParams{
		OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: r.ID, Valid: true}, EventCode: Event,
	})
	if err != nil {
		return nil, err
	}
	byDelivery := map[uuid.UUID][]model.Response{}
	for _, rp := range replies {
		byDelivery[rp.DeliveryUuid] = append(byDelivery[rp.DeliveryUuid], model.Response{
			UUID: rp.Uuid, Channel: rp.Channel, Answer: rp.Answer, Result: rp.Result, Reason: rp.Reason,
			RespondedAt: rp.RespondedAt.Time.UTC(),
		})
	}
	out := make([]model.Attempt, 0, len(rows))
	for _, row := range rows {
		a := model.Attempt{Responses: []model.Response{},
			DispatchUUID: row.DispatchUuid, DispatchStatus: row.DispatchStatus, DispatchMessage: row.DispatchError,
			Status: row.DispatchStatus, Error: row.DispatchError, ScheduledAt: row.ScheduledAt.Time.UTC(),
			CreatedAt: row.ScheduledAt.Time.UTC(),
		}
		a.Round, a.Request, a.Call = requestOf(row.IdempotencyKey)
		if row.DeliveryUuid.Valid {
			id := uuid.UUID(row.DeliveryUuid.Bytes)
			a.DeliveryUUID = &id
			a.Channel = textPtr(row.Channel)
			a.Try = int(row.AttemptNo.Int32)
			a.Status = row.DeliveryStatus.String
			a.Provider = row.Provider.String
			a.Recipient = row.RecipientMasked.String
			a.Error = row.DeliveryError.String
			a.SentAt, a.FinishedAt = timePtr(row.SentAt), timePtr(row.FinishedAt)
			a.DeliveredAt, a.ReadAt = timePtr(row.DeliveredAt), timePtr(row.ReadAt)
			a.CreatedAt = row.DeliveryCreatedAt.Time.UTC()
			if rs := byDelivery[id]; rs != nil {
				a.Responses = rs
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// requestOf reads the round, request number and extra call of a request
// key ("…|r2:a1", "…|r2:a1:c1" for the first call made again).
func requestOf(key string) (round, request, call int) {
	i := strings.LastIndex(key, "|r")
	if i < 0 {
		return 0, 0, 0
	}
	r, a, ok := strings.Cut(key[i+2:], ":a")
	if !ok {
		return 0, 0, 0
	}
	a, c, _ := strings.Cut(a, ":c")
	round, _ = strconv.Atoi(r)
	request, _ = strconv.Atoi(a)
	call, _ = strconv.Atoi(c)
	return round, request, call
}

// ConfirmByPhone records a confirmation the customer gave on the phone
// (TEC-924): the same answer as the customer's "Geleceğim" (no more
// requests, no automatic cancel), with the member in the history. A
// booking outside the caller's branch scope is not found; one that changed
// since it was read (expected version) is stale.
func (e *Engine) ConfirmByPhone(ctx context.Context, orgID int64, id uuid.UUID, actorID int64, in model.ConfirmInput) error {
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > model.MaxChangeNote {
		ve := &ValidationError{}
		ve.add("note", "too_long", "note is at most 500 characters")
		return ve
	}
	err := e.Answer(ctx, orgID, id, AnswerInput{
		Answer: model.AnswerConfirm, Source: model.SourcePhone, Note: note, ActorID: actorID,
		Guard: func(l reservationsmodel.Locked) error {
			if orgctx.CheckBranch(ctx, l.BranchID) != nil {
				return ErrNotFound
			}
			if in.ExpectedVersion != nil && *in.ExpectedVersion != l.Version {
				return reservationsusecase.ErrStaleVersion
			}
			return nil
		},
	})
	return err
}
