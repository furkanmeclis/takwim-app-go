package model

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

// AnswerProblem is one invalid booking form answer (Field is
// custom_fields.<key>).
type AnswerProblem struct {
	Field   string
	Code    string
	Message string
}

// Where a booking form is filled in.
const (
	AskOnline = "online"
	AskPanel  = "panel"
)

// Asked reports whether the question is asked where (AskOnline / AskPanel).
func (f CustomField) Asked(where string) bool {
	if where == AskPanel {
		return f.Panel
	}
	return f.Online
}

// CheckAnswers validates the answers of a booking form filled in where
// (AskOnline / AskPanel) against the business's questions. It returns the
// answers to store and the problems found.
//
//   - A question asked there is checked: a required one must be answered
//     (a required checkbox ticked), a text is at most MaxTextAnswer
//     characters, a number is a finite number, a select one of its options,
//     a checkbox true or false. An empty text or null counts as no answer
//     and is not stored.
//   - Online, keys of other questions are dropped: a customer answers only
//     what the business asks.
//   - In the panel, other keys (questions asked only online, older answers)
//     stay as they are, so editing a booking never loses an answer.
func CheckAnswers(fields []CustomField, in map[string]any, where string) (map[string]any, []AnswerProblem) {
	out := map[string]any{}
	var problems []AnswerProblem
	asked := map[string]bool{}
	for _, f := range fields {
		if !f.Asked(where) {
			continue
		}
		asked[f.Key] = true
		field := "custom_fields." + f.Key
		raw, ok := in[f.Key]
		if s, isString := raw.(string); isString && f.Type != FieldCheckbox {
			raw = strings.TrimSpace(s)
			if raw == "" {
				ok = false
			}
		}
		if raw == nil {
			ok = false
		}
		if !ok {
			if f.Required {
				problems = append(problems, AnswerProblem{Field: field, Code: "required", Message: f.Label + " is required"})
			}
			continue
		}
		switch f.Type {
		case FieldText:
			s, isString := raw.(string)
			switch {
			case !isString:
				problems = append(problems, AnswerProblem{Field: field, Code: "invalid", Message: "answer must be text"})
			case utf8.RuneCountInString(s) > MaxTextAnswer:
				problems = append(problems, AnswerProblem{Field: field, Code: "too_long", Message: fmt.Sprintf("answer must be at most %d characters", MaxTextAnswer)})
			default:
				out[f.Key] = s
			}
		case FieldNumber:
			n, isNumber := raw.(float64)
			if !isNumber || math.IsNaN(n) || math.IsInf(n, 0) {
				problems = append(problems, AnswerProblem{Field: field, Code: "invalid", Message: "answer must be a number"})
				continue
			}
			out[f.Key] = n
		case FieldSelect:
			s, isString := raw.(string)
			if !isString || !slices.Contains(f.Options, s) {
				problems = append(problems, AnswerProblem{Field: field, Code: "invalid", Message: "answer must be one of the options"})
				continue
			}
			out[f.Key] = s
		case FieldCheckbox:
			b, isBool := raw.(bool)
			switch {
			case !isBool:
				problems = append(problems, AnswerProblem{Field: field, Code: "invalid", Message: "answer must be true or false"})
			case f.Required && !b:
				problems = append(problems, AnswerProblem{Field: field, Code: "required", Message: f.Label + " must be accepted"})
			default:
				out[f.Key] = b
			}
		}
	}
	if where == AskPanel {
		for k, v := range in {
			if !asked[k] {
				out[k] = v
			}
		}
	}
	return out, problems
}

// Snapshot returns the definitions of the answered questions: the one a
// record already keeps for a key (previous), else the current question,
// so a record keeps the label it was first answered under.
func Snapshot(fields, previous []CustomField, answers map[string]any) []CustomField {
	out := []CustomField{}
	done := map[string]bool{}
	for _, list := range [][]CustomField{previous, fields} {
		for _, f := range list {
			if _, ok := answers[f.Key]; ok && !done[f.Key] {
				out = append(out, f)
				done[f.Key] = true
			}
		}
	}
	return out
}
