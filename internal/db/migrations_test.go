package db

import (
	"io/fs"
	"strings"
	"testing"
)

// TestMigrationsEmbedded checks that all expected migration files are present
// in the embedded FS and contain the required goose markers. This is a static
// validation — it does not execute the SQL.
func TestMigrationsEmbedded(t *testing.T) {
	expected := []string{
		"00001_users_accounts.sql",
		"00002_campaigns_sequences.sql",
		"00003_chats_messages.sql",
		"00004_ai_safety.sql",
		"00005_safety_limits.sql",
		"00006_funnel_ab.sql",
		"00007_linkedin_search.sql",
		"00008_followup.sql",
		"00009_sync_misc.sql",
		"00010_views_triggers.sql",
	}

	mFS := Migrations()
	found := map[string]bool{}

	err := fs.WalkDir(mFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}
		found[path] = true

		data, readErr := fs.ReadFile(mFS, path)
		if readErr != nil {
			return readErr
		}
		body := string(data)
		if !strings.Contains(body, "-- +goose Up") {
			t.Errorf("%s: missing '-- +goose Up' marker", path)
		}
		if !strings.Contains(body, "-- +goose Down") {
			t.Errorf("%s: missing '-- +goose Down' marker", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}

	for _, name := range expected {
		if !found[name] {
			t.Errorf("expected migration not found: %s", name)
		}
	}
	if len(found) != len(expected) {
		t.Errorf("migration count mismatch: found=%d expected=%d", len(found), len(expected))
	}
}
