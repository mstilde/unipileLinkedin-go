package sequence

import (
	"strings"
	"testing"
)

func intp(i int) *int { return &i }

func TestValidate_EmptySequence(t *testing.T) {
	err := Validate(nil)
	if err == nil {
		t.Fatal("expected error for nil sequence")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(ve.Errors) == 0 || !strings.Contains(ve.Errors[0], "at least 1") {
		t.Errorf("unexpected error: %v", ve.Errors)
	}
}

func TestValidate_FirstStepCannotBeWait(t *testing.T) {
	steps := []Step{
		{StepType: StepWait, Delay: &Delay{Value: 1, Unit: "hours"}},
	}
	err := Validate(steps)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "first step cannot be") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_UnknownStepType(t *testing.T) {
	steps := []Step{{StepType: StepType("nope")}}
	err := Validate(steps)
	if err == nil || !strings.Contains(err.Error(), `unknown step_type "nope"`) {
		t.Errorf("expected unknown step_type error, got %v", err)
	}
}

func TestValidate_InviteRequiresTemplate(t *testing.T) {
	steps := []Step{{StepType: StepInvite}}
	err := Validate(steps)
	if err == nil || !strings.Contains(err.Error(), "invite requires template") {
		t.Errorf("expected invite template error, got %v", err)
	}
}

func TestValidate_InviteCharLimit(t *testing.T) {
	cases := []struct {
		limit   int
		wantErr bool
	}{
		{1, false},
		{200, false},
		{300, false},
		{0, true},
		{301, true},
		{-5, true},
	}
	for _, tc := range cases {
		steps := []Step{
			{StepType: StepInvite, Template: "hi", CharLimit: intp(tc.limit)},
		}
		err := Validate(steps)
		hasErr := err != nil && strings.Contains(err.Error(), "char_limit out of range")
		if hasErr != tc.wantErr {
			t.Errorf("limit=%d wantErr=%v gotErr=%v (err=%v)", tc.limit, tc.wantErr, hasErr, err)
		}
	}
}

func TestValidate_NoteMaxCharsAsAlias(t *testing.T) {
	// note_max_chars should be honored when char_limit is absent
	steps := []Step{
		{StepType: StepInvite, Template: "hi", NoteMaxChars: intp(150)},
	}
	if err := Validate(steps); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestValidate_MessageRequiresTemplate(t *testing.T) {
	for _, st := range []StepType{StepMessage, StepSendMessage} {
		err := Validate([]Step{{StepType: st}})
		if err == nil || !strings.Contains(err.Error(), "message requires template") {
			t.Errorf("type=%s: expected template error, got %v", st, err)
		}
	}
}

func TestValidate_InMailRequiresTemplateAndSubject(t *testing.T) {
	steps := []Step{
		{StepType: StepInvite, Template: "hi"},
		{StepType: StepInMail},
	}
	err := Validate(steps)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "inmail requires template") {
		t.Errorf("missing template error: %s", msg)
	}
	if !strings.Contains(msg, "inmail requires subject") {
		t.Errorf("missing subject error: %s", msg)
	}
}

func TestValidate_VoiceNoteEitherURLOrTemplate(t *testing.T) {
	steps := []Step{
		{StepType: StepInvite, Template: "hi"},
		{StepType: StepVoiceNote},
	}
	if err := Validate(steps); err == nil ||
		!strings.Contains(err.Error(), "voice_note requires") {
		t.Errorf("expected voice_note error, got %v", err)
	}

	// With audio_url should pass
	steps[1] = Step{StepType: StepVoiceNote, AudioURL: "https://x/audio.mp3"}
	if err := Validate(steps); err != nil {
		t.Errorf("expected valid with audio_url, got %v", err)
	}

	// With ai_voice_template should pass
	steps[1] = Step{StepType: StepVoiceNote, AIVoiceTemplate: "Hi {{firstName}}"}
	if err := Validate(steps); err != nil {
		t.Errorf("expected valid with ai_voice_template, got %v", err)
	}
}

func TestValidate_LikePostSelector(t *testing.T) {
	cases := []struct {
		sel     string
		wantErr bool
	}{
		{"", false}, // defaults to "last_post"
		{"last_post", false},
		{"last_n_posts", false},
		{"last_n_posts:3", false},
		{"last_n_posts:abc", true},
		{"weirdo", true},
	}
	for _, tc := range cases {
		steps := []Step{
			{StepType: StepInvite, Template: "hi"},
			{StepType: StepLikePost, PostSelector: tc.sel},
		}
		err := Validate(steps)
		hasErr := err != nil && strings.Contains(err.Error(), "invalid post_selector")
		if hasErr != tc.wantErr {
			t.Errorf("sel=%q wantErr=%v gotErr=%v err=%v", tc.sel, tc.wantErr, hasErr, err)
		}
	}
}

func TestValidate_WaitNeedsDelay(t *testing.T) {
	steps := []Step{
		{StepType: StepInvite, Template: "hi"},
		{StepType: StepWait}, // missing delay/delay_hours
	}
	if err := Validate(steps); err == nil ||
		!strings.Contains(err.Error(), "wait requires delay") {
		t.Errorf("expected wait delay error, got %v", err)
	}
}

func TestValidate_WaitValueAndUnit(t *testing.T) {
	steps := []Step{
		{StepType: StepInvite, Template: "hi"},
		{StepType: StepWait, Delay: &Delay{Value: -1, Unit: "weeks"}},
	}
	err := Validate(steps)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "value must be >= 0") {
		t.Errorf("missing value error: %s", msg)
	}
	if !strings.Contains(msg, "unit must be minutes|hours|days") {
		t.Errorf("missing unit error: %s", msg)
	}
}

func TestValidate_ConditionBranching(t *testing.T) {
	// Loop: branch points back to current index
	steps := []Step{
		{StepType: StepInvite, Template: "hi"},
		{StepType: StepCondition, Predicate: "always_true", BranchIfTrue: []int{1}},
		{StepType: StepMessage, Template: "x"},
	}
	err := Validate(steps)
	if err == nil || !strings.Contains(err.Error(), "would create loop") {
		t.Errorf("expected loop error, got %v", err)
	}

	// Out of range
	steps[1] = Step{StepType: StepCondition, Predicate: "always_true", BranchIfTrue: []int{99}}
	err = Validate(steps)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected out-of-range error, got %v", err)
	}

	// Unknown predicate
	steps[1] = Step{StepType: StepCondition, Predicate: "weird_pred", BranchIfTrue: []int{2}}
	err = Validate(steps)
	if err == nil || !strings.Contains(err.Error(), `unknown predicate "weird_pred"`) {
		t.Errorf("expected predicate error, got %v", err)
	}

	// Valid predicate with parens
	steps[1] = Step{StepType: StepCondition, Predicate: "accepted_invite_within_days(7)", BranchIfTrue: []int{2}, BranchIfFalse: []int{2}}
	if err := Validate(steps); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestValidate_ABTest(t *testing.T) {
	// Less than 2 variants
	steps := []Step{
		{StepType: StepABTest, Variants: []Variant{{ID: "A", Template: "x", Weight: 50}}},
	}
	if err := Validate(steps); err == nil || !strings.Contains(err.Error(), "at least 2 variants") {
		t.Errorf("expected variants error, got %v", err)
	}

	// Duplicate IDs
	steps[0] = Step{
		StepType: StepABTest,
		Variants: []Variant{
			{ID: "A", Template: "x", Weight: 50},
			{ID: "A", Template: "y", Weight: 50},
		},
	}
	if err := Validate(steps); err == nil || !strings.Contains(err.Error(), `duplicate variant id "A"`) {
		t.Errorf("expected duplicate id error, got %v", err)
	}

	// Total weight 0
	steps[0] = Step{
		StepType: StepABTest,
		Variants: []Variant{
			{ID: "A", Template: "x", Weight: 0},
			{ID: "B", Template: "y", Weight: 0},
		},
	}
	if err := Validate(steps); err == nil || !strings.Contains(err.Error(), "total weight must be > 0") {
		t.Errorf("expected total weight error, got %v", err)
	}

	// Invalid metric
	steps[0] = Step{
		StepType: StepABTest,
		Variants: []Variant{
			{ID: "A", Template: "x", Weight: 50},
			{ID: "B", Template: "y", Weight: 50},
		},
		Metric: "garbage",
	}
	if err := Validate(steps); err == nil || !strings.Contains(err.Error(), `invalid metric "garbage"`) {
		t.Errorf("expected metric error, got %v", err)
	}

	// Valid
	steps[0] = Step{
		StepType: StepABTest,
		Variants: []Variant{
			{ID: "A", Template: "x", Weight: 50},
			{ID: "B", Template: "y", Weight: 50},
		},
		Metric: "reply_rate",
	}
	if err := Validate(steps); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestValidate_HappyPath(t *testing.T) {
	steps := []Step{
		{StepType: StepInvite, Template: "Hi {{firstName|amigo}}", CharLimit: intp(200)},
		{StepType: StepWait, Delay: &Delay{Value: 24, Unit: "hours", SkipWeekends: true}},
		{StepType: StepSendMessage, Template: "Hi again"},
		{StepType: StepWait, Delay: &Delay{Value: 3, Unit: "days"}},
		{StepType: StepSendMessage, Template: "Last one"},
	}
	if err := Validate(steps); err != nil {
		t.Errorf("expected valid sequence, got %v", err)
	}
}

func TestIsValidationError(t *testing.T) {
	err := Validate(nil)
	if !IsValidationError(err) {
		t.Errorf("expected IsValidationError true, got false (err=%v)", err)
	}
}
