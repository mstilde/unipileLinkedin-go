// Package delay computes next-run times for scheduled actions, respecting
// timezone, working hours, weekends, holidays, and human-jitter randomization.
//
// Direct port of lib/delay-engine.js from the Node original. Two main entry
// points:
//
//   - ComputeNextRunTime: applies a delay (value+unit), optional randomization,
//     and optional working-hours rules to a base time, returning the resulting
//     time in the configured IANA timezone.
//   - ExponentialBackoffMinutes: classic exponential backoff with jitter, for
//     retry scheduling. Supports deterministic jitter per accountId so
//     concurrent failures of N accounts don't pile up at the same retry time.
package delay

import (
	"hash/fnv"
	"math"
	"math/rand"
	"time"
)

// Unit is the time unit of a delay value.
type Unit string

const (
	UnitMinutes Unit = "minutes"
	UnitHours   Unit = "hours"
	UnitDays    Unit = "days"
)

// Randomize describes jitter applied to a delay.
//
//   - Uniform (default): factor = MinPct + (MaxPct-MinPct) * rng()
//   - Gaussian:          factor centered at midpoint(MinPct,MaxPct), half-spread,
//     gaussian rng clipped to [-1, 1].
//
// MinPct and MaxPct are percent values (e.g. -20, +30); the delay becomes
// baseMs * (1 + factor/100).
type Randomize struct {
	MinPct       float64
	MaxPct       float64
	Distribution string // "" / "uniform" / "gaussian"
}

// WorkingHours defines the open-for-business window inside a day, by hour.
// Start is inclusive, End is exclusive. Defaults: 9..19.
type WorkingHours struct {
	Start int
	End   int
}

// LunchBreak defines a midday pause treated like off-hours.
// If Start >= End or unset, lunch is disabled.
type LunchBreak struct {
	Start int
	End   int
}

// Config carries all the rules used by ComputeNextRunTime.
type Config struct {
	Value     int  // amount of Unit
	Unit      Unit // minutes | hours | days (default: hours)
	Randomize *Randomize

	RespectWorkingHours bool
	WorkingHours        WorkingHours
	LunchBreak          *LunchBreak
	SkipWeekends        bool
	SkipHolidays        []string // YYYY-MM-DD strings in the TZ
}

// RNG is an optional source of randomness; if nil, rand.Float64 is used.
type RNG func() float64

// ComputeNextRunTime computes the next-run time of a scheduled action.
//
//   - base: starting point (must be a real instant; the TZ is taken from `tz` string).
//   - cfg: delay + working-hours rules.
//   - tz: IANA timezone string (e.g. "America/Mexico_City"). If invalid, falls
//     back to UTC.
//   - rng: source of randomness; nil → rand.Float64.
//
// Returns the resulting time in the requested TZ.
func ComputeNextRunTime(base time.Time, cfg Config, tz string, rng RNG) time.Time {
	if rng == nil {
		rng = rand.Float64
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		loc = time.UTC
	}

	dur := durationFor(cfg.Value, cfg.Unit)
	if cfg.Randomize != nil {
		dur = applyRandomization(dur, cfg.Randomize, rng)
	}
	next := base.Add(dur).In(loc)

	if cfg.RespectWorkingHours {
		next = moveToNextWorkingSlot(next, cfg, loc)
	} else if cfg.SkipWeekends || len(cfg.SkipHolidays) > 0 {
		next = skipNonWorkingDays(next, cfg, loc)
	}
	return next
}

func durationFor(value int, unit Unit) time.Duration {
	switch unit {
	case UnitMinutes:
		return time.Duration(value) * time.Minute
	case UnitDays:
		return time.Duration(value) * 24 * time.Hour
	case UnitHours, "":
		return time.Duration(value) * time.Hour
	default:
		return time.Duration(value) * time.Hour
	}
}

func applyRandomization(base time.Duration, r *Randomize, rng RNG) time.Duration {
	min := r.MinPct / 100.0
	max := r.MaxPct / 100.0
	var factor float64
	if r.Distribution == "gaussian" {
		center := (min + max) / 2.0
		half := (max - min) / 2.0
		factor = center + half*gaussianRandom(rng)
	} else {
		factor = min + (max-min)*rng()
	}
	return time.Duration(float64(base) * (1.0 + factor))
}

// gaussianRandom returns a value in [-1, 1] (Box-Muller, clipped to 2σ).
func gaussianRandom(rng RNG) float64 {
	u1 := math.Max(rng(), 1e-9)
	u2 := rng()
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	if z < -2 {
		z = -2
	}
	if z > 2 {
		z = 2
	}
	return z / 2.0
}

// IsWeekendInTZ reports whether t falls on Sat/Sun in the given timezone.
func IsWeekendInTZ(t time.Time, tz string) bool {
	loc, _ := time.LoadLocation(tz)
	if loc == nil {
		loc = time.UTC
	}
	wd := t.In(loc).Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// IsHolidayInTZ reports whether t (in TZ) is in the holidays list (YYYY-MM-DD).
func IsHolidayInTZ(t time.Time, tz string, holidays []string) bool {
	if len(holidays) == 0 {
		return false
	}
	loc, _ := time.LoadLocation(tz)
	if loc == nil {
		loc = time.UTC
	}
	ymd := t.In(loc).Format("2006-01-02")
	for _, h := range holidays {
		if h == ymd {
			return true
		}
	}
	return false
}

// IsWithinWorkingHours reports whether t (in TZ) falls in [start, end).
// Defaults: start=9, end=19.
func IsWithinWorkingHours(t time.Time, tz string, wh WorkingHours) bool {
	loc, _ := time.LoadLocation(tz)
	if loc == nil {
		loc = time.UTC
	}
	start, end := wh.Start, wh.End
	if start == 0 && end == 0 {
		start, end = 9, 19
	}
	hour := t.In(loc).Hour()
	return hour >= start && hour < end
}

// IsWithinLunchBreak reports whether t (in TZ) falls in the lunch window.
func IsWithinLunchBreak(t time.Time, tz string, lb *LunchBreak) bool {
	if lb == nil || lb.Start >= lb.End {
		return false
	}
	loc, _ := time.LoadLocation(tz)
	if loc == nil {
		loc = time.UTC
	}
	hour := t.In(loc).Hour()
	return hour >= lb.Start && hour < lb.End
}

// moveToNextWorkingSlot moves t forward until it satisfies the working-hours,
// lunch, weekend and holiday rules. Bounded by an iteration safety limit
// (200 iterations) to defend against config bugs.
func moveToNextWorkingSlot(t time.Time, cfg Config, loc *time.Location) time.Time {
	start, end := cfg.WorkingHours.Start, cfg.WorkingHours.End
	if start == 0 && end == 0 {
		start, end = 9, 19
	}
	if start >= end {
		return t
	}

	cur := t.In(loc)
	tzName := loc.String()
	for i := 0; i < 200; i++ {
		// Off-day: weekend (if enabled) or holiday
		if (cfg.SkipWeekends && (cur.Weekday() == time.Saturday || cur.Weekday() == time.Sunday)) ||
			IsHolidayInTZ(cur, tzName, cfg.SkipHolidays) {
			cur = time.Date(cur.Year(), cur.Month(), cur.Day()+1, start, 0, 0, 0, loc)
			continue
		}
		hour := cur.Hour()
		if hour < start {
			cur = time.Date(cur.Year(), cur.Month(), cur.Day(), start, 0, 0, 0, loc)
			continue
		}
		if hour >= end {
			cur = time.Date(cur.Year(), cur.Month(), cur.Day()+1, start, 0, 0, 0, loc)
			continue
		}
		if IsWithinLunchBreak(cur, tzName, cfg.LunchBreak) {
			cur = time.Date(cur.Year(), cur.Month(), cur.Day(), cfg.LunchBreak.End, 0, 0, 0, loc)
			continue
		}
		return cur
	}
	return cur
}

// skipNonWorkingDays advances t by 1 day at a time as long as it lands on a
// weekend (when SkipWeekends) or holiday. Used when RespectWorkingHours is
// false but we still want to dodge non-working days.
func skipNonWorkingDays(t time.Time, cfg Config, loc *time.Location) time.Time {
	tzName := loc.String()
	cur := t.In(loc)
	for i := 0; i < 30; i++ {
		isWeekend := cfg.SkipWeekends && (cur.Weekday() == time.Saturday || cur.Weekday() == time.Sunday)
		isHoliday := IsHolidayInTZ(cur, tzName, cfg.SkipHolidays)
		if !isWeekend && !isHoliday {
			return cur
		}
		cur = cur.Add(24 * time.Hour)
	}
	return cur
}

// ----------------------------------------------------------------------------
// Exponential backoff with jitter (retry scheduling)
// ----------------------------------------------------------------------------

// BackoffKind identifies one of the preset backoff curves.
type BackoffKind string

const (
	BackoffThrottle  BackoffKind = "throttle"   // 5m → 6h
	BackoffTransient BackoffKind = "transient"  // 5m → 4h
	BackoffRateLimit BackoffKind = "rate_limit" // 1h → 24h
)

// BackoffOptions configures ExponentialBackoffMinutes.
type BackoffOptions struct {
	Kind        BackoffKind
	BaseMinutes int     // 0 → defaults from Kind
	MaxMinutes  int     // 0 → defaults from Kind
	JitterPct   float64 // 0 → 0.2 (±20%)
	AccountID   string  // if non-empty, jitter is deterministic per accountId+attempt
}

// ExponentialBackoffMinutes returns the next retry delay in minutes,
// computed as: min(baseMin * 2^attempt, maxMin) * (1 ± jitterPct).
//
// When opts.AccountID is set, jitter is derived from a FNV hash of
// "accountId|kind|attempt" so two accounts that fail at the same time get
// different retry slots (no thundering herd).
//
// The returned value is always >= 1.
func ExponentialBackoffMinutes(attempt int, opts BackoffOptions) int {
	defaults := map[BackoffKind]struct{ base, max int }{
		BackoffThrottle:  {5, 360},
		BackoffTransient: {5, 240},
		BackoffRateLimit: {60, 1440},
	}
	kind := opts.Kind
	if kind == "" {
		kind = BackoffTransient
	}
	def, ok := defaults[kind]
	if !ok {
		def = defaults[BackoffTransient]
	}
	baseMin := def.base
	if opts.BaseMinutes > 0 {
		baseMin = opts.BaseMinutes
	}
	maxMin := def.max
	if opts.MaxMinutes > 0 {
		maxMin = opts.MaxMinutes
	}
	jitter := 0.2
	if opts.JitterPct != 0 {
		jitter = opts.JitterPct
	}
	if attempt < 0 {
		attempt = 0
	}

	expMin := float64(baseMin) * math.Pow(2, float64(attempt))
	if expMin > float64(maxMin) {
		expMin = float64(maxMin)
	}

	var factor float64
	if opts.AccountID != "" {
		h := fnv.New32a()
		_, _ = h.Write([]byte(opts.AccountID))
		_, _ = h.Write([]byte("|"))
		_, _ = h.Write([]byte(string(kind)))
		_, _ = h.Write([]byte("|"))
		// itoa without strconv import (and to mirror JS behavior)
		_, _ = h.Write([]byte(itoa(attempt)))
		norm := float64(h.Sum32()) / float64(0xffffffff)
		factor = 1.0 + (norm*2.0-1.0)*jitter
	} else {
		factor = 1.0 + (rand.Float64()*2.0-1.0)*jitter
	}

	out := int(math.Floor(expMin * factor))
	if out < 1 {
		out = 1
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
