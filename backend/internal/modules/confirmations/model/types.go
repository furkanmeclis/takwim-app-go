// Package model holds the types of the customer confirmation module
// (TEC-921, the Konfirmasyon add-on): the settings that decide when and how
// a customer is asked to confirm a booking, and the answers.
package model

import (
	"time"

	"github.com/google/uuid"
)

// What happens when the customer does not answer by the deadline.
const (
	// OnNoAnswerCancel cancels the booking and frees its capacity (K-007).
	OnNoAnswerCancel = "cancel"
	// OnNoAnswerFlag keeps the booking and marks the confirmation expired;
	// the business decides.
	OnNoAnswerFlag = "flag"
)

// Task statuses (reservation_confirmations.status).
const (
	TaskScheduled = "scheduled"
	TaskPending   = "pending"
	TaskConfirmed = "confirmed"
	TaskDeclined  = "declined"
	TaskExpired   = "expired"
	// TaskDropped: the task no longer applies (the visit was cancelled,
	// started, finished or moved past the deadline, or the add-on or the
	// branch setting was turned off).
	TaskDropped = "dropped"
)

// Outcomes of an expired task.
const (
	OutcomeCancelled = "cancelled"
	OutcomeFlagged   = "flagged"
	// OutcomeUnreachable: no request could be sent on any channel; the
	// booking is never cancelled for a message the customer did not get.
	OutcomeUnreachable = "unreachable"
)

// Customer answers.
const (
	AnswerConfirm = "confirm"
	AnswerCancel  = "cancel"
	AnswerChange  = "change"
)

// Where an answer came from.
const (
	SourcePanel     = "panel"
	SourceVisitLink = "visit_link"
)

// Limits of the settings.
const (
	// MinResponseMinutes is the least time a customer gets between a
	// request and the deadline.
	MinResponseMinutes = 30
	// MinFirstRequestMinutes / MaxMinutes bound the timings (minutes
	// before the visit start).
	MinFirstRequestMinutes = 30
	MaxMinutes             = 7 * 24 * 60
	// MaxAttempts bounds the requests of one booking (repeat limit).
	MaxAttempts = 5
	// MinWindowMinutes is the shortest contact window.
	MinWindowMinutes = 60
	// MaxChangeNote bounds the customer's change request note.
	MaxChangeNote = 500
)

// Rule is when and how customers of a branch are asked to confirm.
type Rule struct {
	// Enabled: bookings of the branch ask for a confirmation.
	Enabled bool `json:"enabled"`
	// Channels in order: the first is the primary channel, the others are
	// tried when it fails (Fallback).
	Channels []string `json:"channels"`
	Fallback bool     `json:"fallback"`
	// FirstRequestMinutes before the start the first request goes out
	// (immediately for a booking made later).
	FirstRequestMinutes int `json:"first_request_minutes"`
	// FollowUpMinutes before the start the follow-up goes out (0 = none).
	FollowUpMinutes int `json:"follow_up_minutes"`
	// MaxAttempts is the number of requests, the first one included;
	// follow-ups beyond the first are spread until the deadline.
	MaxAttempts int `json:"max_attempts"`
	// DeadlineMinutes before the start is the last moment an answer is
	// waited for.
	DeadlineMinutes int `json:"deadline_minutes"`
	// WindowStartMinute / WindowEndMinute are the local hours (minutes of
	// the day) requests and calls go out; nil = any time.
	WindowStartMinute *int   `json:"window_start_minute"`
	WindowEndMinute   *int   `json:"window_end_minute"`
	OnNoAnswer        string `json:"on_no_answer"`
}

func intPtr(v int) *int { return &v }

// DefaultRule is the starting template (requirements §9): first request
// 24 hours, follow-up 3 hours, last answer 2 hours before; SMS then
// e-mail; requests between 09:00 and 21:00; cancel on no answer.
func DefaultRule() Rule {
	return Rule{
		Enabled: true, Channels: []string{"sms", "email"}, Fallback: true,
		FirstRequestMinutes: 24 * 60, FollowUpMinutes: 180, MaxAttempts: 2, DeadlineMinutes: 120,
		WindowStartMinute: intPtr(9 * 60), WindowEndMinute: intPtr(21 * 60), OnNoAnswer: OnNoAnswerCancel,
	}
}

// Ref names a branch.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Channel is a channel the settings may use, with its readiness.
type Channel struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// BranchRule is the effective rule of one branch.
type BranchRule struct {
	Branch Ref `json:"branch"`
	// Custom: the branch has its own rule; otherwise it follows the
	// business-wide rule.
	Custom    bool       `json:"custom"`
	Rule      Rule       `json:"rule"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// Limits are the bounds the settings page applies.
type Limits struct {
	MinResponseMinutes     int `json:"min_response_minutes"`
	MinFirstRequestMinutes int `json:"min_first_request_minutes"`
	MaxMinutes             int `json:"max_minutes"`
	MaxAttempts            int `json:"max_attempts"`
	MinWindowMinutes       int `json:"min_window_minutes"`
}

// Settings is the confirmation settings page.
type Settings struct {
	Channels []Channel `json:"channels"`
	// Default is the business-wide rule (the template when DefaultCustom
	// is false).
	Default          Rule         `json:"default"`
	DefaultCustom    bool         `json:"default_custom"`
	DefaultUpdatedAt *time.Time   `json:"default_updated_at"`
	Template         Rule         `json:"template"`
	Branches         []BranchRule `json:"branches"`
	Limits           Limits       `json:"limits"`
}

// SaveInput replaces the business-wide rule (BranchUUID nil) or a
// branch's rule.
type SaveInput struct {
	BranchUUID *uuid.UUID `json:"branch_uuid"`
	Rule
}

// ResetInput removes the business-wide rule (back to the template) or a
// branch's rule (back to the business-wide one).
type ResetInput struct {
	BranchUUID *uuid.UUID `json:"branch_uuid"`
}

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Code    string
	Message string
}
