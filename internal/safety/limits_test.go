package safety

import (
	"testing"
	"time"
)

func TestDefaultCapFor_TierAware(t *testing.T) {
	if got := DefaultCapFor(ActionInvite, TierFree); got != 10 {
		t.Errorf("free invite: got %d want 10", got)
	}
	if got := DefaultCapFor(ActionInvite, TierPremium); got != 15 {
		t.Errorf("premium invite: got %d want 15", got)
	}
	if got := DefaultCapFor(ActionInMail, TierSalesNav); got != 50 {
		t.Errorf("sales_nav inmail: got %d want 50", got)
	}
	if got := DefaultCapFor(ActionInMail, TierRecruiter); got != 100 {
		t.Errorf("recruiter inmail: got %d want 100", got)
	}
}

func TestDefaultCapFor_NoTier_UsesLegacy(t *testing.T) {
	if got := DefaultCapFor(ActionInvite, ""); got != 20 {
		t.Errorf("legacy invite: got %d want 20", got)
	}
}

func TestDefaultCapFor_UnknownAction_Fallback(t *testing.T) {
	if got := DefaultCapFor(Action("weird"), ""); got != 20 {
		t.Errorf("unknown: got %d want 20", got)
	}
}

func TestDefaultWeeklyCapFor(t *testing.T) {
	if got := DefaultWeeklyCapFor(ActionInvite); got != 80 {
		t.Errorf("invite: got %d", got)
	}
	if got := DefaultWeeklyCapFor(ActionVisit); got != 0 {
		t.Errorf("visit (no weekly cap): got %d want 0", got)
	}
}

func TestRampPctForDay_DefaultCurve(t *testing.T) {
	curve := DefaultRampCurve()
	cases := []struct {
		day  int
		want int
	}{
		{1, 30}, {3, 30}, {4, 60}, {7, 60}, {8, 100}, {30, 100},
	}
	for _, c := range cases {
		if got := RampPctForDay(c.day, curve); got != c.want {
			t.Errorf("day=%d: got %d want %d", c.day, got, c.want)
		}
	}
}

func TestEffectiveCap_NoRamp(t *testing.T) {
	if got := EffectiveCap(20, false, time.Time{}, RampCurve{}, time.Now()); got != 20 {
		t.Errorf("no ramp: got %d want 20", got)
	}
}

func TestEffectiveCap_LinearRamp(t *testing.T) {
	start := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	curve := LemListRampCurve() // start=2, increment=2
	// Day 1 → 2; day 2 → 4; day 5 → 10; capped at rawCap=8
	now := time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC) // day 1
	if got := EffectiveCap(8, true, start, curve, now); got != 2 {
		t.Errorf("day 1: got %d want 2", got)
	}
	now = start.Add(48 * time.Hour) // day 3
	if got := EffectiveCap(8, true, start, curve, now); got != 6 {
		t.Errorf("day 3: got %d want 6", got)
	}
	now = start.Add(7 * 24 * time.Hour) // day 8, computed > rawCap
	if got := EffectiveCap(8, true, start, curve, now); got != 8 {
		t.Errorf("day 8 (cap): got %d want 8", got)
	}
}

func TestEffectiveCap_StaircaseRamp(t *testing.T) {
	start := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	curve := DefaultRampCurve()
	rawCap := 100

	// Day 1 → 30% → 30
	now := start.Add(time.Hour)
	if got := EffectiveCap(rawCap, true, start, curve, now); got != 30 {
		t.Errorf("day 1: got %d want 30", got)
	}
	// Day 5 → 60% → 60
	now = start.Add(4 * 24 * time.Hour)
	if got := EffectiveCap(rawCap, true, start, curve, now); got != 60 {
		t.Errorf("day 5: got %d want 60", got)
	}
	// Day 10 → 100% → 100
	now = start.Add(9 * 24 * time.Hour)
	if got := EffectiveCap(rawCap, true, start, curve, now); got != 100 {
		t.Errorf("day 10: got %d want 100", got)
	}
}

func TestEffectiveCap_AlwaysAtLeast1(t *testing.T) {
	start := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	curve := RampCurve{Linear: &RampLinear{Start: 0, Increment: 0}}
	if got := EffectiveCap(100, true, start, curve, start); got != 1 {
		t.Errorf("min: got %d want 1", got)
	}
}

func TestNextMondayMorning_OnSunday(t *testing.T) {
	sun := time.Date(2026, 5, 24, 15, 0, 0, 0, time.UTC) // Sunday
	mon := NextMondayMorning(sun, "")
	if mon.Weekday() != time.Monday {
		t.Errorf("got weekday %v", mon.Weekday())
	}
	if mon.Day() != 25 {
		t.Errorf("day got %d want 25", mon.Day())
	}
	if mon.Hour() != 9 || mon.Minute() != 0 {
		t.Errorf("time got %02d:%02d, want 09:00", mon.Hour(), mon.Minute())
	}
}

func TestNextMondayMorning_OnMonday_JumpsToNext(t *testing.T) {
	mon1 := time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC) // Monday
	mon2 := NextMondayMorning(mon1, "")
	if mon2.Weekday() != time.Monday {
		t.Errorf("weekday got %v", mon2.Weekday())
	}
	if mon2.Sub(mon1) < 24*time.Hour {
		t.Errorf("should jump to next Monday, got delta %v", mon2.Sub(mon1))
	}
}

func TestNextMondayMorning_DeterministicJitterPerAccount(t *testing.T) {
	sun := time.Date(2026, 5, 24, 15, 0, 0, 0, time.UTC)
	a := NextMondayMorning(sun, "acct-A")
	b := NextMondayMorning(sun, "acct-A")
	if !a.Equal(b) {
		t.Errorf("same accountID should yield same time, got %v vs %v", a, b)
	}
	c := NextMondayMorning(sun, "acct-B")
	if a.Equal(c) {
		t.Errorf("different accountIDs should differ (got same: %v)", a)
	}
}

func TestNextMondayMorning_JitterInWindow(t *testing.T) {
	sun := time.Date(2026, 5, 24, 15, 0, 0, 0, time.UTC)
	// Try 20 accountIDs and check all land in [08:30, 11:30)
	for i := 0; i < 20; i++ {
		mon := NextMondayMorning(sun, itoa(i))
		mins := mon.Hour()*60 + mon.Minute()
		if mins < 8*60+30 || mins >= 11*60+30 {
			t.Errorf("account %d: got %02d:%02d outside [08:30, 11:30)", i, mon.Hour(), mon.Minute())
		}
	}
}

func TestDaysSince(t *testing.T) {
	start := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	if got := DaysSince(start, start); got != 1 {
		t.Errorf("same day: got %d want 1", got)
	}
	if got := DaysSince(start, start.Add(48*time.Hour)); got != 3 {
		t.Errorf("after 2 days: got %d want 3", got)
	}
	if got := DaysSince(time.Time{}, start); got != 1 {
		t.Errorf("zero start: got %d want 1", got)
	}
}
