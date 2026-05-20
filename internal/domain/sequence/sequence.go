// Package sequence contains the campaign sequence domain types and validator.
//
// The validator enforces the same rules as lib/sequence-validator.js from the
// Node original:
//
//   - Sequence must have at least one step
//   - First step type must be in FirstStepAllowed
//   - Each step type must be in KnownStepTypes
//   - condition.branch_if_* indices must point to valid, later steps (no loops)
//   - ab_test must have >= 2 variants with unique IDs and positive total weight
//   - invite.char_limit must be in [1, 300]
//   - wait.delay must have value >= 0 and unit in {minutes, hours, days}
//   - voice_note requires either audio_url or ai_voice_template
//   - inmail requires both template and subject
package sequence

import (
	"errors"
	"fmt"
	"strings"
)

// StepType identifies what a sequence step does.
type StepType string

const (
	StepInvite         StepType = "invite"
	StepMessage        StepType = "message"
	StepSendMessage    StepType = "send_message"
	StepVisitProfile   StepType = "visit_profile"
	StepFollow         StepType = "follow"
	StepLikePost       StepType = "like_post"
	StepCommentPost    StepType = "comment_post"
	StepWithdrawInvite StepType = "withdraw_invite"
	StepVoiceNote      StepType = "voice_note"
	StepInMail         StepType = "inmail"
	StepWait           StepType = "wait"
	StepCondition      StepType = "condition"
	StepABTest         StepType = "ab_test"
	StepEnd            StepType = "end"
)

// KnownStepTypes is the set of accepted step_type values.
var KnownStepTypes = map[StepType]struct{}{
	StepInvite: {}, StepMessage: {}, StepSendMessage: {},
	StepVisitProfile: {}, StepFollow: {}, StepLikePost: {},
	StepCommentPost: {}, StepWithdrawInvite: {}, StepVoiceNote: {},
	StepInMail: {}, StepWait: {}, StepCondition: {},
	StepABTest: {}, StepEnd: {},
}

// FirstStepAllowed restricts what types may appear as step[0].
var FirstStepAllowed = map[StepType]struct{}{
	StepInvite: {}, StepVisitProfile: {}, StepFollow: {},
	StepSendMessage: {}, StepMessage: {}, StepABTest: {},
}

// KnownPredicates is the set of accepted condition.predicate names.
var KnownPredicates = map[string]struct{}{
	"accepted_invite_within_days": {},
	"replied_within_days":         {},
	"visited_profile_within_days": {},
	"tag_equals":                  {},
	"always_true":                 {},
	"always_false":                {},
}

// Step is the in-memory representation of a sequence step.
// Fields are loosely typed because step_type controls which fields apply; the
// validator does the type-specific enforcement.
type Step struct {
	StepType StepType `json:"step_type"`

	// invite / message / inmail / comment_post / voice_note(ai)
	Template      string `json:"template,omitempty"`
	AIPersonalize bool   `json:"ai_personalize,omitempty"`
	CharLimit     *int   `json:"char_limit,omitempty"`
	NoteMaxChars  *int   `json:"note_max_chars,omitempty"`

	// inmail
	Subject string `json:"subject,omitempty"`

	// voice_note
	AudioURL        string `json:"audio_url,omitempty"`
	AIVoiceTemplate string `json:"ai_voice_template,omitempty"`

	// like_post / comment_post
	PostSelector string `json:"post_selector,omitempty"`
	PostCount    *int   `json:"post_count,omitempty"`

	// withdraw_invite
	IfNotAcceptedAfterDays *int `json:"if_not_accepted_after_days,omitempty"`

	// wait
	Delay      *Delay `json:"delay,omitempty"`
	DelayHours *int   `json:"delay_hours,omitempty"`

	// condition
	Predicate     string `json:"predicate,omitempty"`
	WithinDays    *int   `json:"within_days,omitempty"`
	BranchIfTrue  []int  `json:"branch_if_true,omitempty"`
	BranchIfFalse []int  `json:"branch_if_false,omitempty"`

	// ab_test
	Variants []Variant `json:"variants,omitempty"`
	Metric   string    `json:"metric,omitempty"`

	// end
	Outcome string `json:"outcome,omitempty"`
}

// Delay configures a wait step.
type Delay struct {
	Value               int        `json:"value"`
	Unit                string     `json:"unit,omitempty"` // minutes | hours | days
	Randomize           *Randomize `json:"randomize,omitempty"`
	RespectWorkingHours bool       `json:"respect_working_hours,omitempty"`
	SkipWeekends        bool       `json:"skip_weekends,omitempty"`
	SkipHolidays        []string   `json:"skip_holidays,omitempty"`
}

// Randomize describes how to jitter a delay.
type Randomize struct {
	MinPct       float64 `json:"min_pct,omitempty"`
	MaxPct       float64 `json:"max_pct,omitempty"`
	Distribution string  `json:"distribution,omitempty"` // uniform (default) | gaussian
}

// Variant is one arm of an A/B test step.
type Variant struct {
	ID       string  `json:"id"`
	Weight   float64 `json:"weight"` // <0 invalid; sum must be > 0
	Template string  `json:"template"`
}

// ValidationError carries a list of human-readable problems with the sequence.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid sequence: %s", strings.Join(e.Errors, "; "))
}

// Validate returns nil if steps is valid; otherwise *ValidationError with all
// problems found. It never returns a non-nil non-*ValidationError.
func Validate(steps []Step) error {
	var errs []string

	if len(steps) == 0 {
		errs = append(errs, "sequence must have at least 1 step")
		return &ValidationError{Errors: errs}
	}

	total := len(steps)
	for i, step := range steps {
		ctx := fmt.Sprintf("step[%d]", i)
		stepType := step.StepType

		if stepType == "" {
			errs = append(errs, ctx+": missing step_type")
			continue
		}
		if _, ok := KnownStepTypes[stepType]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown step_type %q", ctx, stepType))
			continue
		}
		if i == 0 {
			if _, ok := FirstStepAllowed[stepType]; !ok {
				errs = append(errs, fmt.Sprintf("%s: first step cannot be %q", ctx, stepType))
			}
		}

		switch stepType {
		case StepInvite:
			if step.Template == "" {
				errs = append(errs, ctx+": invite requires template")
			}
			limit := 200
			if step.CharLimit != nil {
				limit = *step.CharLimit
			} else if step.NoteMaxChars != nil {
				limit = *step.NoteMaxChars
			}
			if limit < 1 || limit > 300 {
				errs = append(errs, ctx+": invite char_limit out of range (1-300)")
			}

		case StepMessage, StepSendMessage:
			if step.Template == "" {
				errs = append(errs, ctx+": message requires template")
			}

		case StepCommentPost:
			if step.Template == "" {
				errs = append(errs, ctx+": comment_post requires template")
			}

		case StepInMail:
			if step.Template == "" {
				errs = append(errs, ctx+": inmail requires template")
			}
			if step.Subject == "" {
				errs = append(errs, ctx+": inmail requires subject")
			}

		case StepVoiceNote:
			if step.AudioURL == "" && step.AIVoiceTemplate == "" {
				errs = append(errs, ctx+": voice_note requires audio_url or ai_voice_template")
			}

		case StepLikePost:
			sel := step.PostSelector
			if sel == "" {
				sel = "last_post"
			}
			if !isValidPostSelector(sel) {
				errs = append(errs, fmt.Sprintf("%s: invalid post_selector %q", ctx, sel))
			}
			if sel == "last_n_posts" && step.PostCount != nil && *step.PostCount < 1 {
				errs = append(errs, ctx+": post_count must be positive integer")
			}

		case StepWait:
			if step.Delay == nil && step.DelayHours == nil {
				errs = append(errs, ctx+": wait requires delay or delay_hours")
			}
			if step.Delay != nil {
				if step.Delay.Value < 0 {
					errs = append(errs, ctx+": wait.delay.value must be >= 0")
				}
				if step.Delay.Unit != "" && !isValidUnit(step.Delay.Unit) {
					errs = append(errs, ctx+": wait.delay.unit must be minutes|hours|days")
				}
			}

		case StepCondition:
			if step.Predicate == "" {
				errs = append(errs, ctx+": condition requires predicate")
			} else {
				predName := step.Predicate
				if idx := strings.IndexByte(predName, '('); idx > 0 {
					predName = predName[:idx]
				}
				if _, ok := KnownPredicates[predName]; !ok {
					errs = append(errs, fmt.Sprintf("%s: unknown predicate %q", ctx, predName))
				}
			}
			for _, idx := range step.BranchIfTrue {
				if idx < 0 || idx >= total {
					errs = append(errs, fmt.Sprintf("%s: branch index %d out of range", ctx, idx))
				} else if idx <= i {
					errs = append(errs, fmt.Sprintf("%s: branch index %d would create loop (must be > current %d)", ctx, idx, i))
				}
			}
			for _, idx := range step.BranchIfFalse {
				if idx < 0 || idx >= total {
					errs = append(errs, fmt.Sprintf("%s: branch index %d out of range", ctx, idx))
				} else if idx <= i {
					errs = append(errs, fmt.Sprintf("%s: branch index %d would create loop (must be > current %d)", ctx, idx, i))
				}
			}

		case StepABTest:
			if len(step.Variants) < 2 {
				errs = append(errs, ctx+": ab_test requires at least 2 variants")
			} else {
				seenIDs := make(map[string]struct{}, len(step.Variants))
				totalW := 0.0
				for vi, v := range step.Variants {
					if v.ID == "" {
						errs = append(errs, fmt.Sprintf("%s: variant[%d] missing id", ctx, vi))
					} else if _, dup := seenIDs[v.ID]; dup {
						errs = append(errs, fmt.Sprintf("%s: duplicate variant id %q", ctx, v.ID))
					} else {
						seenIDs[v.ID] = struct{}{}
					}
					if v.Template == "" {
						errs = append(errs, fmt.Sprintf("%s: variant[%d] missing template", ctx, vi))
					}
					if v.Weight < 0 {
						errs = append(errs, fmt.Sprintf("%s: variant[%d] weight must be >= 0", ctx, vi))
					}
					totalW += v.Weight
				}
				if totalW <= 0 {
					errs = append(errs, ctx+": ab_test variants total weight must be > 0")
				}
			}
			if step.Metric != "" && !isValidMetric(step.Metric) {
				errs = append(errs, fmt.Sprintf("%s: ab_test invalid metric %q", ctx, step.Metric))
			}
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// MustValidate panics on invalid sequences. Useful in tests / fixtures.
func MustValidate(steps []Step) {
	if err := Validate(steps); err != nil {
		panic(err)
	}
}

// IsValidationError reports whether err is a *ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

func isValidPostSelector(s string) bool {
	if s == "last_post" || s == "last_n_posts" {
		return true
	}
	if strings.HasPrefix(s, "last_n_posts:") {
		num := s[len("last_n_posts:"):]
		if num == "" {
			return false
		}
		for _, r := range num {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	return false
}

func isValidUnit(s string) bool {
	return s == "minutes" || s == "hours" || s == "days"
}

func isValidMetric(s string) bool {
	return s == "reply_rate" || s == "accept_rate" || s == "click_rate"
}
