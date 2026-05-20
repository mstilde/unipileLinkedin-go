// Package safety contains pure-logic safety primitives that protect the
// LinkedIn account from triggering platform bans: daily / weekly caps with
// tier-aware defaults, ramp-up curves, deterministic next-Monday-morning
// scheduling for weekly-cap retries, A/B variant picking, blacklist matching
// and short-name extraction.
//
// All functions are pure (no DB, no HTTP) so they can be tested without
// infrastructure. DB-backed wrappers live elsewhere.
package safety

import (
	"hash/fnv"
	"strconv"
	"time"
)

// Tier is the LinkedIn account tier. Different tiers get different caps.
type Tier string

const (
	TierFree      Tier = "free"
	TierPremium   Tier = "premium"
	TierSalesNav  Tier = "sales_nav"
	TierRecruiter Tier = "recruiter"
)

// Action is one of the platform actions we rate-limit.
type Action string

const (
	ActionInvite          Action = "invite"
	ActionMessage         Action = "message"
	ActionSendMessage     Action = "send_message"
	ActionVisit           Action = "visit"
	ActionVisitProfile    Action = "visit_profile"
	ActionFollow          Action = "follow"
	ActionLike            Action = "like"
	ActionLikePost        Action = "like_post"
	ActionComment         Action = "comment"
	ActionCommentPost     Action = "comment_post"
	ActionInMail          Action = "inmail"
	ActionVoiceNote       Action = "voice_note"
	ActionWithdrawInvite  Action = "withdraw_invite"
	ActionEngagementNoise Action = "engagement_noise"
)

// DefaultCaps are the legacy non-tier-aware caps. Used as last-resort fallback.
var DefaultCaps = map[Action]int{
	ActionInvite: 20, ActionMessage: 40, ActionSendMessage: 40,
	ActionVisit: 80, ActionVisitProfile: 80,
	ActionFollow: 30, ActionLike: 50, ActionLikePost: 50,
	ActionComment: 15, ActionCommentPost: 15,
	ActionInMail: 20, ActionVoiceNote: 10,
	ActionWithdrawInvite: 100, ActionEngagementNoise: 5,
}

// TierCaps are tier-aware daily caps. Mirrors TIER_CAPS in account-limits.js.
var TierCaps = map[Tier]map[Action]int{
	TierFree: {
		ActionInvite: 10, ActionMessage: 20, ActionSendMessage: 20,
		ActionVisit: 40, ActionVisitProfile: 40,
		ActionFollow: 20, ActionLike: 30, ActionLikePost: 30,
		ActionComment: 10, ActionCommentPost: 10,
		ActionInMail: 0, ActionVoiceNote: 0,
		ActionWithdrawInvite: 50, ActionEngagementNoise: 3,
	},
	TierPremium: {
		ActionInvite: 15, ActionMessage: 40, ActionSendMessage: 40,
		ActionVisit: 80, ActionVisitProfile: 80,
		ActionFollow: 30, ActionLike: 50, ActionLikePost: 50,
		ActionComment: 15, ActionCommentPost: 15,
		ActionInMail: 20, ActionVoiceNote: 10,
		ActionWithdrawInvite: 100, ActionEngagementNoise: 5,
	},
	TierSalesNav: {
		ActionInvite: 20, ActionMessage: 60, ActionSendMessage: 60,
		ActionVisit: 150, ActionVisitProfile: 150,
		ActionFollow: 30, ActionLike: 50, ActionLikePost: 50,
		ActionComment: 15, ActionCommentPost: 15,
		ActionInMail: 50, ActionVoiceNote: 10,
		ActionWithdrawInvite: 100, ActionEngagementNoise: 5,
	},
	TierRecruiter: {
		ActionInvite: 25, ActionMessage: 80, ActionSendMessage: 80,
		ActionVisit: 200, ActionVisitProfile: 200,
		ActionFollow: 30, ActionLike: 50, ActionLikePost: 50,
		ActionComment: 15, ActionCommentPost: 15,
		ActionInMail: 100, ActionVoiceNote: 10,
		ActionWithdrawInvite: 100, ActionEngagementNoise: 5,
	},
}

// DefaultWeeklyCaps are 7-day rolling caps. Soft caps below LinkedIn's hard cap.
// A nil-or-zero value means "no weekly cap for this action".
var DefaultWeeklyCaps = map[Action]int{
	ActionInvite:    80,
	ActionInMail:    200,
	ActionMessage:   700,
	ActionVoiceNote: 50,
}

// DefaultCapFor returns the per-day cap for (action, tier). If tier is empty,
// uses the legacy DefaultCaps. If both miss, returns 20 (safe default for invite).
func DefaultCapFor(action Action, tier Tier) int {
	if tier != "" {
		if t, ok := TierCaps[tier]; ok {
			if v, ok := t[action]; ok {
				return v
			}
		}
	}
	if v, ok := DefaultCaps[action]; ok {
		return v
	}
	return 20
}

// DefaultWeeklyCapFor returns the rolling 7-day cap for an action. Zero means
// "no weekly cap configured" — callers should treat this as unlimited.
func DefaultWeeklyCapFor(action Action) int {
	return DefaultWeeklyCaps[action]
}

// RampCurve is either a staircase percentage curve or a linear LemList-style
// curve. Exactly one of (Stairs, Linear) should be set.
type RampCurve struct {
	Stairs []RampStair  // staircase: %cap per day-range
	Linear *RampLinear  // linear: start + (day-1)*increment, capped at rawCap
}

// RampStair is one segment of a staircase ramp: from day_from to day_to (nil = ∞).
type RampStair struct {
	DayFrom int
	DayTo   *int // nil = open-ended
	Pct     int
}

// RampLinear configures a LemList-style linear ramp.
type RampLinear struct {
	Start     int // day 1 cap
	Increment int // added per subsequent day
}

// DefaultRampCurve is the staircase default: 30% days 1-3, 60% days 4-7, 100% days 8+.
func DefaultRampCurve() RampCurve {
	dayTo3 := 3
	dayTo7 := 7
	return RampCurve{
		Stairs: []RampStair{
			{DayFrom: 1, DayTo: &dayTo3, Pct: 30},
			{DayFrom: 4, DayTo: &dayTo7, Pct: 60},
			{DayFrom: 8, DayTo: nil, Pct: 100},
		},
	}
}

// LemListRampCurve is the linear default used by the seed_account_daily_limits trigger.
func LemListRampCurve() RampCurve {
	return RampCurve{Linear: &RampLinear{Start: 2, Increment: 2}}
}

// RampPctForDay returns the % of rawCap applicable on the given ramp day (1-indexed).
// Curve being zero-value (no stairs and no linear) defaults to 100%.
func RampPctForDay(day int, curve RampCurve) int {
	if len(curve.Stairs) == 0 {
		return 100
	}
	for _, s := range curve.Stairs {
		if day >= s.DayFrom && (s.DayTo == nil || day <= *s.DayTo) {
			return s.Pct
		}
	}
	return 100
}

// DaysSince returns the 1-indexed day number since startedAt (day 1 = same day).
// Returns 1 when startedAt is zero.
func DaysSince(startedAt, now time.Time) int {
	if startedAt.IsZero() {
		return 1
	}
	d := int(now.Sub(startedAt).Hours()/24) + 1
	if d < 1 {
		return 1
	}
	return d
}

// EffectiveCap returns the cap to enforce today, given the raw cap and an
// optional ramp curve. When rampEnabled is false (or rampStartedAt is zero),
// returns rawCap unchanged.
func EffectiveCap(rawCap int, rampEnabled bool, rampStartedAt time.Time, curve RampCurve, now time.Time) int {
	if !rampEnabled || rampStartedAt.IsZero() {
		return rawCap
	}
	day := DaysSince(rampStartedAt, now)

	if curve.Linear != nil {
		start := max1(curve.Linear.Start)
		inc := curve.Linear.Increment
		if inc < 0 {
			inc = 0
		}
		computed := start + (day-1)*inc
		if computed > rawCap {
			computed = rawCap
		}
		return max1(computed)
	}

	pct := RampPctForDay(day, curve)
	out := rawCap * pct / 100
	return max1(out)
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// NextMondayMorning returns the next Monday after `now` at a jittered time
// between 08:30 and 11:30 UTC. The jitter is deterministic per (accountId,
// Monday-date) so two failing accounts don't pile up at the exact same minute
// (no thundering herd), but retries for the same account land in the same slot
// (idempotent).
//
// If accountID is empty, returns 09:00:00 UTC (legacy behavior).
func NextMondayMorning(now time.Time, accountID string) time.Time {
	d := now.UTC()
	dow := int(d.Weekday()) // Sunday=0, Monday=1, ...
	days := (1 - dow + 7) % 7
	if days == 0 {
		days = 7 // if today is Monday, jump to next Monday
	}
	monday := time.Date(d.Year(), d.Month(), d.Day()+days, 9, 0, 0, 0, time.UTC)

	if accountID == "" {
		return monday
	}

	seed := accountID + "|" + monday.Format("2006-01-02")
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	offsetMin := int(h.Sum32() % 180) // 0..179 → 08:30..11:29
	totalMin := 8*60 + 30 + offsetMin
	hour := totalMin / 60
	minute := totalMin % 60
	return time.Date(monday.Year(), monday.Month(), monday.Day(), hour, minute, 0, 0, time.UTC)
}

// itoa wrapper kept consistent for tests / debugging.
func itoa(n int) string { return strconv.Itoa(n) }
