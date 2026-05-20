package delay

import (
	"math"
	"testing"
	"time"
)

const TZ = "America/Argentina/Buenos_Aires"

func mustParse(t *testing.T, value, tz string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	tm, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func fixedRNG(v float64) RNG { return func() float64 { return v } }

func TestComputeNextRunTime_SimpleHours(t *testing.T) {
	base := mustParse(t, "2026-05-20 10:00", TZ)
	got := ComputeNextRunTime(base, Config{Value: 5, Unit: UnitHours}, TZ, nil)
	want := base.Add(5 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestComputeNextRunTime_Days(t *testing.T) {
	base := mustParse(t, "2026-05-20 10:00", TZ)
	got := ComputeNextRunTime(base, Config{Value: 3, Unit: UnitDays}, TZ, nil)
	if !got.Equal(base.Add(72 * time.Hour)) {
		t.Errorf("got %v", got)
	}
}

func TestComputeNextRunTime_Minutes(t *testing.T) {
	base := mustParse(t, "2026-05-20 10:00", TZ)
	got := ComputeNextRunTime(base, Config{Value: 45, Unit: UnitMinutes}, TZ, nil)
	if !got.Equal(base.Add(45 * time.Minute)) {
		t.Errorf("got %v", got)
	}
}

func TestComputeNextRunTime_RandomizeUniform(t *testing.T) {
	base := mustParse(t, "2026-05-20 10:00", TZ)
	cfg := Config{
		Value: 10, Unit: UnitHours,
		Randomize: &Randomize{MinPct: 0, MaxPct: 20},
	}
	got := ComputeNextRunTime(base, cfg, TZ, fixedRNG(0.5))
	wantDur := 11 * time.Hour
	if math.Abs(float64(got.Sub(base)-wantDur)) > float64(time.Minute) {
		t.Errorf("got delta %v want ~%v", got.Sub(base), wantDur)
	}
}

func TestComputeNextRunTime_RandomizeNegative(t *testing.T) {
	base := mustParse(t, "2026-05-20 10:00", TZ)
	cfg := Config{
		Value: 10, Unit: UnitHours,
		Randomize: &Randomize{MinPct: -20, MaxPct: 0},
	}
	got := ComputeNextRunTime(base, cfg, TZ, fixedRNG(0))
	wantDur := 8 * time.Hour
	if math.Abs(float64(got.Sub(base)-wantDur)) > float64(time.Minute) {
		t.Errorf("got delta %v want ~%v", got.Sub(base), wantDur)
	}
}

func TestComputeNextRunTime_WorkingHours_OutsideRange_MovesToNext(t *testing.T) {
	base := mustParse(t, "2026-05-18 22:00", TZ)
	cfg := Config{
		Value: 4, Unit: UnitHours,
		RespectWorkingHours: true,
		WorkingHours:        WorkingHours{Start: 9, End: 19},
	}
	got := ComputeNextRunTime(base, cfg, TZ, nil)
	loc, _ := time.LoadLocation(TZ)
	want := time.Date(2026, 5, 19, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestComputeNextRunTime_WorkingHours_BeforeStart_MovesForward(t *testing.T) {
	base := mustParse(t, "2026-05-20 06:00", TZ)
	cfg := Config{
		Value: 1, Unit: UnitHours,
		RespectWorkingHours: true,
		WorkingHours:        WorkingHours{Start: 9, End: 19},
	}
	got := ComputeNextRunTime(base, cfg, TZ, nil)
	loc, _ := time.LoadLocation(TZ)
	want := time.Date(2026, 5, 20, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestComputeNextRunTime_SkipWeekend(t *testing.T) {
	base := mustParse(t, "2026-05-22 18:00", TZ)
	if base.Weekday() != time.Friday {
		t.Fatalf("expected Friday, got %v", base.Weekday())
	}
	cfg := Config{
		Value: 24, Unit: UnitHours,
		SkipWeekends: true,
	}
	got := ComputeNextRunTime(base, cfg, TZ, nil)
	if got.Weekday() != time.Monday {
		t.Errorf("got weekday %v want Monday", got.Weekday())
	}
}

func TestComputeNextRunTime_SkipHoliday(t *testing.T) {
	base := mustParse(t, "2026-05-20 10:00", TZ)
	cfg := Config{
		Value: 24, Unit: UnitHours,
		SkipHolidays: []string{"2026-05-21"},
	}
	got := ComputeNextRunTime(base, cfg, TZ, nil)
	if got.Day() != 22 {
		t.Errorf("got day %d want 22 (Friday, after Thursday holiday)", got.Day())
	}
}

func TestComputeNextRunTime_WorkingHours_LunchBreak(t *testing.T) {
	base := mustParse(t, "2026-05-20 13:30", TZ)
	cfg := Config{
		Value: 0, Unit: UnitHours,
		RespectWorkingHours: true,
		WorkingHours:        WorkingHours{Start: 9, End: 19},
		LunchBreak:          &LunchBreak{Start: 13, End: 14},
	}
	got := ComputeNextRunTime(base, cfg, TZ, nil)
	loc, _ := time.LoadLocation(TZ)
	want := time.Date(2026, 5, 20, 14, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestIsWeekendInTZ(t *testing.T) {
	loc, _ := time.LoadLocation(TZ)
	sat := time.Date(2026, 5, 23, 12, 0, 0, 0, loc)
	if !IsWeekendInTZ(sat, TZ) {
		t.Error("Saturday should be weekend")
	}
	mon := time.Date(2026, 5, 25, 12, 0, 0, 0, loc)
	if IsWeekendInTZ(mon, TZ) {
		t.Error("Monday should not be weekend")
	}
}

func TestIsHolidayInTZ(t *testing.T) {
	loc, _ := time.LoadLocation(TZ)
	d := time.Date(2026, 5, 25, 12, 0, 0, 0, loc)
	if !IsHolidayInTZ(d, TZ, []string{"2026-05-25"}) {
		t.Error("expected holiday match")
	}
	if IsHolidayInTZ(d, TZ, []string{"2026-12-25"}) {
		t.Error("expected no match")
	}
}

func TestIsWithinWorkingHours(t *testing.T) {
	loc, _ := time.LoadLocation(TZ)
	wh := WorkingHours{Start: 9, End: 19}
	cases := []struct {
		hour int
		want bool
	}{
		{8, false},
		{9, true},
		{12, true},
		{18, true},
		{19, false},
		{22, false},
	}
	for _, tc := range cases {
		ts := time.Date(2026, 5, 20, tc.hour, 0, 0, 0, loc)
		got := IsWithinWorkingHours(ts, TZ, wh)
		if got != tc.want {
			t.Errorf("hour=%d got=%v want=%v", tc.hour, got, tc.want)
		}
	}
}

func TestExponentialBackoff_Defaults(t *testing.T) {
	got := ExponentialBackoffMinutes(0, BackoffOptions{Kind: BackoffTransient, AccountID: "acct1"})
	if got < 4 || got > 6 {
		t.Errorf("attempt=0 transient: got %d, want ~5 (4-6)", got)
	}
	got = ExponentialBackoffMinutes(2, BackoffOptions{Kind: BackoffTransient, AccountID: "acct1"})
	if got < 16 || got > 24 {
		t.Errorf("attempt=2 transient: got %d, want ~20 (16-24)", got)
	}
}

func TestExponentialBackoff_Capped(t *testing.T) {
	got := ExponentialBackoffMinutes(10, BackoffOptions{Kind: BackoffTransient, AccountID: "acct1"})
	if got > 300 {
		t.Errorf("got %d, should be capped near 240", got)
	}
	if got < 190 {
		t.Errorf("got %d, should be near cap 240", got)
	}
}

func TestExponentialBackoff_AccountIDDeterministic(t *testing.T) {
	a := ExponentialBackoffMinutes(3, BackoffOptions{Kind: BackoffTransient, AccountID: "acct-A"})
	b := ExponentialBackoffMinutes(3, BackoffOptions{Kind: BackoffTransient, AccountID: "acct-A"})
	if a != b {
		t.Errorf("same accountID+attempt should yield same delay: %d vs %d", a, b)
	}
	c := ExponentialBackoffMinutes(3, BackoffOptions{Kind: BackoffTransient, AccountID: "acct-B"})
	if a == c {
		t.Errorf("different accountIDs should yield different delays (got %d for both)", a)
	}
}

func TestExponentialBackoff_RateLimitKind(t *testing.T) {
	got := ExponentialBackoffMinutes(0, BackoffOptions{Kind: BackoffRateLimit, AccountID: "acct1"})
	if got < 48 || got > 72 {
		t.Errorf("rate_limit attempt=0: got %d, want ~60 (48-72)", got)
	}
}

func TestExponentialBackoff_AlwaysAtLeast1(t *testing.T) {
	got := ExponentialBackoffMinutes(0, BackoffOptions{
		Kind: BackoffTransient, BaseMinutes: 1, MaxMinutes: 1, JitterPct: 0.99, AccountID: "x",
	})
	if got < 1 {
		t.Errorf("must be >= 1, got %d", got)
	}
}
