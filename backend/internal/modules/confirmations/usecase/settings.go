package usecase

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// Resource is the activity resource of the settings.
const Resource = "tenant.confirmations"

// ErrBranchNotFound: the branch is missing, deleted or outside the
// caller's branch scope.
var ErrBranchNotFound = errors.New("branch not found")

// ValidationError carries field details; handlers answer 400.
type ValidationError struct {
	Details []model.Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, model.Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

// Channels are the channels a confirmation may use, in display order (the
// automatic voice call included, TEC-923).
var Channels = channel.Known

// windowField checks a pair of window bounds (minutes of the day).
func windowField(ve *ValidationError, prefix string, start, end *int) {
	switch {
	case (start == nil) != (end == nil):
		ve.add(prefix+"window_end_minute", "required", "the window needs a start and an end")
	case start != nil:
		s, e := *start, *end
		switch {
		case s < 0 || s > 1439:
			ve.add(prefix+"window_start_minute", "out_of_range", "window start must be a time of the day")
		case e < 1 || e > 1440:
			ve.add(prefix+"window_end_minute", "out_of_range", "window end must be a time of the day")
		case e-s < model.MinWindowMinutes:
			ve.add(prefix+"window_end_minute", "too_short", "the window must be at least an hour")
		}
	}
}

// Validate checks a rule: known channels, timings that leave the customer
// time to answer and a deadline that is not after the start (requirements
// §9: settings that break this are not saved).
func Validate(r model.Rule) error {
	ve := &ValidationError{}
	if len(r.Channels) == 0 {
		ve.add("channels", "required", "at least one channel is required")
	}
	seen := map[string]bool{}
	for i, c := range r.Channels {
		field := "channels." + strconv.Itoa(i)
		switch {
		case !slices.Contains(Channels, c):
			ve.add(field, "invalid", "unknown channel")
		case seen[c]:
			ve.add(field, "duplicate", "channel listed twice")
		}
		seen[c] = true
	}
	response := model.MinResponseMinutes
	switch {
	case r.FirstRequestMinutes < model.MinFirstRequestMinutes || r.FirstRequestMinutes > model.MaxMinutes:
		ve.add("first_request_minutes", "out_of_range", "first request must be 30 minutes to 7 days before the visit")
	case r.DeadlineMinutes < 0 || r.DeadlineMinutes > model.MaxMinutes:
		ve.add("deadline_minutes", "out_of_range", "the deadline must be at or before the visit start")
	case r.FirstRequestMinutes-r.DeadlineMinutes < response:
		ve.add("deadline_minutes", "no_response_time", "the deadline must leave at least 30 minutes after the first request")
	}
	switch {
	case r.FollowUpMinutes < 0 || r.FollowUpMinutes > model.MaxMinutes:
		ve.add("follow_up_minutes", "out_of_range", "follow-up must be within 7 days before the visit")
	case r.FollowUpMinutes > 0 && r.FollowUpMinutes >= r.FirstRequestMinutes:
		ve.add("follow_up_minutes", "after_first", "the follow-up must come after the first request")
	case r.FollowUpMinutes > 0 && r.FollowUpMinutes-r.DeadlineMinutes < response:
		ve.add("follow_up_minutes", "no_response_time", "the follow-up must leave at least 30 minutes before the deadline")
	}
	switch {
	case r.MaxAttempts < 1 || r.MaxAttempts > model.MaxAttempts:
		ve.add("max_attempts", "out_of_range", "attempts must be 1 to 5")
	case r.MaxAttempts > 1 && r.FollowUpMinutes == 0:
		ve.add("max_attempts", "needs_follow_up", "more than one attempt needs a follow-up time")
	}
	windowField(ve, "", r.WindowStartMinute, r.WindowEndMinute)
	windowField(ve, "call_", r.CallWindowStartMinute, r.CallWindowEndMinute)
	// Calls must have time to ring inside the request hours.
	if slices.Contains(r.Channels, channel.VoiceCall) && r.WindowStartMinute != nil && r.WindowEndMinute != nil &&
		r.CallWindowStartMinute != nil && r.CallWindowEndMinute != nil &&
		min(*r.WindowEndMinute, *r.CallWindowEndMinute)-max(*r.WindowStartMinute, *r.CallWindowStartMinute) < model.MinWindowMinutes {
		ve.add("call_window_end_minute", "no_overlap", "call hours must overlap the request hours by at least an hour")
	}
	if r.CallRetries < 0 || r.CallRetries > model.MaxCallRetries {
		ve.add("call_retries", "out_of_range", "extra calls must be 0 to 3")
	}
	if r.CallRetryMinutes < model.MinCallRetryMinutes || r.CallRetryMinutes > model.MaxCallRetryMinutes {
		ve.add("call_retry_minutes", "out_of_range", "the wait before calling again must be 15 minutes to 4 hours")
	}
	if r.OnNoAnswer != model.OnNoAnswerCancel && r.OnNoAnswer != model.OnNoAnswerFlag {
		ve.add("on_no_answer", "invalid", "on_no_answer must be cancel or flag")
	}
	return ve.orNil()
}

func ruleOf(row db.ConfirmationSetting) model.Rule {
	r := model.Rule{
		Enabled: row.Enabled, Channels: row.Channels, Fallback: row.Fallback,
		FirstRequestMinutes: int(row.FirstRequestMinutes), FollowUpMinutes: int(row.FollowUpMinutes),
		MaxAttempts: int(row.MaxAttempts), DeadlineMinutes: int(row.DeadlineMinutes), OnNoAnswer: row.OnNoAnswer,
	}
	if r.Channels == nil {
		r.Channels = []string{}
	}
	if row.WindowStartMinute.Valid && row.WindowEndMinute.Valid {
		s, e := int(row.WindowStartMinute.Int32), int(row.WindowEndMinute.Int32)
		r.WindowStartMinute, r.WindowEndMinute = &s, &e
	}
	r.CallRetries, r.CallRetryMinutes = int(row.CallRetries), int(row.CallRetryMinutes)
	if row.CallWindowStartMinute.Valid && row.CallWindowEndMinute.Valid {
		s, e := int(row.CallWindowStartMinute.Int32), int(row.CallWindowEndMinute.Int32)
		r.CallWindowStartMinute, r.CallWindowEndMinute = &s, &e
	}
	return r
}

func int4(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}

func updated(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

// RuleFor is the effective rule of a branch: its own, else the business's,
// else the template.
func RuleFor(ctx context.Context, q *db.Queries, orgID, branchID int64) (model.Rule, error) {
	row, err := q.GetEffectiveConfirmationSettings(ctx, db.GetEffectiveConfirmationSettingsParams{
		OrganizationID: orgID, BranchID: pgtype.Int8{Int64: branchID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.DefaultRule(), nil
	}
	if err != nil {
		return model.Rule{}, err
	}
	return ruleOf(row), nil
}

// Settings answers the settings page: channels, the business-wide rule and
// every branch of the caller's scope with its effective rule.
func (e *Engine) Settings(ctx context.Context, scope orgctx.Scope) (model.Settings, error) {
	rows, err := e.q.ListConfirmationSettings(ctx, scope.ID)
	if err != nil {
		return model.Settings{}, err
	}
	out := model.Settings{
		Default: model.DefaultRule(), Template: model.DefaultRule(), Branches: []model.BranchRule{},
		Limits: model.Limits{
			MinResponseMinutes: model.MinResponseMinutes, MinFirstRequestMinutes: model.MinFirstRequestMinutes,
			MaxMinutes: model.MaxMinutes, MaxAttempts: model.MaxAttempts, MinWindowMinutes: model.MinWindowMinutes,
			MaxCallRetries: model.MaxCallRetries, MinCallRetryMinutes: model.MinCallRetryMinutes,
			MaxCallRetryMinutes: model.MaxCallRetryMinutes,
		},
	}
	for _, c := range Channels {
		out.Channels = append(out.Channels, model.Channel{Name: c, Ready: e.readyFor(ctx, scope.ID, c)})
	}
	byBranch := map[int64]db.ListConfirmationSettingsRow{}
	for _, r := range rows {
		if !r.BranchID.Valid {
			out.Default, out.DefaultCustom, out.DefaultUpdatedAt = ruleOf(settingOf(r)), true, updated(r.UpdatedAt)
			continue
		}
		byBranch[r.BranchID.Int64] = r
	}
	branches, err := e.q.ListConfirmationBranches(ctx, db.ListConfirmationBranchesParams{OrganizationID: scope.ID})
	if err != nil {
		return model.Settings{}, err
	}
	for _, b := range branches {
		if !scope.AllowsBranch(b.ID) {
			continue
		}
		item := model.BranchRule{Branch: model.Ref{UUID: b.Uuid, Name: b.Name}, Rule: out.Default}
		if r, ok := byBranch[b.ID]; ok {
			item.Custom, item.Rule, item.UpdatedAt = true, ruleOf(settingOf(r)), updated(r.UpdatedAt)
		}
		out.Branches = append(out.Branches, item)
	}
	return out, nil
}

func settingOf(r db.ListConfirmationSettingsRow) db.ConfirmationSetting {
	return db.ConfirmationSetting{
		ID: r.ID, OrganizationID: r.OrganizationID, BranchID: r.BranchID, Enabled: r.Enabled, Channels: r.Channels,
		Fallback: r.Fallback, FirstRequestMinutes: r.FirstRequestMinutes, FollowUpMinutes: r.FollowUpMinutes,
		MaxAttempts: r.MaxAttempts, DeadlineMinutes: r.DeadlineMinutes, WindowStartMinute: r.WindowStartMinute,
		WindowEndMinute: r.WindowEndMinute, OnNoAnswer: r.OnNoAnswer, UpdatedBy: r.UpdatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, CallWindowStartMinute: r.CallWindowStartMinute,
		CallWindowEndMinute: r.CallWindowEndMinute, CallRetries: r.CallRetries, CallRetryMinutes: r.CallRetryMinutes,
	}
}

// branchID resolves the branch of a save or reset: nil = the business-wide
// rule, which needs every branch in the caller's scope.
func (e *Engine) branchID(ctx context.Context, scope orgctx.Scope, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil {
		if !scope.AllBranches() {
			return pgtype.Int8{}, ErrBranchNotFound
		}
		return pgtype.Int8{}, nil
	}
	b, err := e.q.GetConfirmationBranch(ctx, db.GetConfirmationBranchParams{OrganizationID: scope.ID, Uuid: *id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !scope.AllowsBranch(b.ID)) {
		return pgtype.Int8{}, ErrBranchNotFound
	}
	if err != nil {
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: b.ID, Valid: true}, nil
}

// Save stores a rule. Existing tasks keep the rule they were planned with;
// new and moved bookings use the new one.
func (e *Engine) Save(ctx context.Context, scope orgctx.Scope, actorID int64, in model.SaveInput) (model.Settings, error) {
	if in.CallRetryMinutes == 0 {
		// A client that does not know the call rules yet.
		in.CallRetryMinutes = model.DefaultCallRetryMinutes
	}
	if err := Validate(in.Rule); err != nil {
		return model.Settings{}, err
	}
	branch, err := e.branchID(ctx, scope, in.BranchUUID)
	if err != nil {
		return model.Settings{}, err
	}
	p := db.UpsertConfirmationSettingsParams{
		OrganizationID: scope.ID, BranchID: branch, Enabled: in.Enabled, Channels: in.Channels, Fallback: in.Fallback,
		FirstRequestMinutes: int32(in.FirstRequestMinutes), FollowUpMinutes: int32(in.FollowUpMinutes),
		MaxAttempts: int32(in.MaxAttempts), DeadlineMinutes: int32(in.DeadlineMinutes), OnNoAnswer: in.OnNoAnswer,
		UpdatedBy:             pgtype.Int8{Int64: actorID, Valid: actorID > 0},
		CallWindowStartMinute: int4(in.CallWindowStartMinute), CallWindowEndMinute: int4(in.CallWindowEndMinute),
		CallRetries: int32(in.CallRetries), CallRetryMinutes: int32(in.CallRetryMinutes),
	}
	if in.WindowStartMinute != nil && in.WindowEndMinute != nil {
		p.WindowStartMinute = pgtype.Int4{Int32: int32(*in.WindowStartMinute), Valid: true}
		p.WindowEndMinute = pgtype.Int4{Int32: int32(*in.WindowEndMinute), Valid: true}
	}
	if _, err := e.q.UpsertConfirmationSettings(ctx, p); err != nil {
		return model.Settings{}, err
	}
	return e.Settings(ctx, scope)
}

// Reset removes a rule: a branch follows the business-wide rule again, the
// business-wide rule goes back to the template.
func (e *Engine) Reset(ctx context.Context, scope orgctx.Scope, in model.ResetInput) (model.Settings, error) {
	branch, err := e.branchID(ctx, scope, in.BranchUUID)
	if err != nil {
		return model.Settings{}, err
	}
	if _, err := e.q.DeleteConfirmationSettings(ctx, db.DeleteConfirmationSettingsParams{OrganizationID: scope.ID, BranchID: branch}); err != nil {
		return model.Settings{}, err
	}
	return e.Settings(ctx, scope)
}
