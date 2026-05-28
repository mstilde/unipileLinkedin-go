package scheduler

import (
	"testing"
	"time"
)

func TestConfigWithDefaults(t *testing.T) {
	c := Config{}.withDefaults()
	if c.CampaignInterval != 15*time.Minute {
		t.Errorf("campaign default: got %v", c.CampaignInterval)
	}
	if c.FollowUpInterval != 15*time.Minute {
		t.Errorf("followup default: got %v", c.FollowUpInterval)
	}
	if c.AIQueueInterval != 30*time.Second {
		t.Errorf("aiqueue default: got %v", c.AIQueueInterval)
	}
	if c.BatchSize != 50 {
		t.Errorf("batch default: got %d", c.BatchSize)
	}
	if c.StaleLeaseAge != 15*time.Minute {
		t.Errorf("stale default: got %v", c.StaleLeaseAge)
	}
}

func TestConfigWithDefaultsPreservesNonZero(t *testing.T) {
	in := Config{
		CampaignInterval: time.Minute,
		FollowUpInterval: 2 * time.Minute,
		AIQueueInterval:  3 * time.Second,
		BatchSize:        7,
		StaleLeaseAge:    9 * time.Minute,
		DryRun:           true,
	}
	out := in.withDefaults()
	if out != in {
		t.Errorf("withDefaults mutated non-zero values: %+v", out)
	}
}
