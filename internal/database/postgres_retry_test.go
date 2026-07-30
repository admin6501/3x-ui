package database

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// TestPostgresRetryGivesUpWithTheDriverError is the fix for a panel that
// restarted every 5 seconds forever.
//
// Failing on the first attempt made the process exit with a generic startup
// error, so systemd looped, the journal filled, and the real reason —
// "connection refused" versus "password authentication failed", which need
// completely different responses — was never printed.
func TestPostgresRetryGivesUpWithTheDriverError(t *testing.T) {
	// Shorten the backoff so the test does not wait the real ~70 seconds.
	original := postgresConnectBackoff
	postgresConnectBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { postgresConnectBackoff = original })

	start := time.Now()
	conn, err := openPostgresWithRetry(
		"host=127.0.0.1 port=1 user=nobody password=nothing dbname=nothing sslmode=disable connect_timeout=1",
		&gorm.Config{},
	)
	if err == nil {
		t.Fatalf("connected to a port nothing listens on: %v", conn)
	}
	if !strings.Contains(err.Error(), "attempts") {
		t.Errorf("the error should say how many attempts were made: %v", err)
	}
	// The driver's own message has to survive, or the operator is back to
	// guessing.
	if len(err.Error()) < 40 {
		t.Errorf("the driver error was swallowed: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("took %s; the backoff override was ignored", elapsed)
	}
}

// TestPostgresBackoffIsBounded keeps the retry window from growing into
// something that looks like a hang.
func TestPostgresBackoffIsBounded(t *testing.T) {
	var total time.Duration
	for _, d := range postgresConnectBackoff {
		if d <= 0 {
			t.Errorf("non-positive backoff step %s", d)
		}
		total += d
	}
	if total < 30*time.Second {
		t.Errorf("total backoff %s is too short to cover a database still starting", total)
	}
	if total > 3*time.Minute {
		t.Errorf("total backoff %s is long enough to look like a hang", total)
	}
}
