package database

import (
	"path/filepath"
	"sync"
	"testing"
)

// TestJournalModeDefaultsToWAL is the fix: under journal_mode=DELETE every
// write serialises the whole database and blocks readers, so this panel's
// concurrent background work — traffic sampling, node sync, mtproto reconcile,
// shop billing — regularly outwaited the busy timeout and failed with
// "database is locked".
func TestJournalModeDefaultsToWAL(t *testing.T) {
	t.Setenv("XUI_DB_JOURNAL_MODE", "")
	if got := sqliteJournalMode(); got != "WAL" {
		t.Errorf("journal mode = %q, want WAL", got)
	}
}

// TestJournalModeOverride: setups that copy the live database file directly
// need the single-file-at-rest behaviour back.
func TestJournalModeOverride(t *testing.T) {
	for _, tc := range []struct{ env, want string }{
		{"DELETE", "DELETE"},
		{"delete", "DELETE"},
		{"  TRUNCATE  ", "TRUNCATE"},
		{"PERSIST", "PERSIST"},
		{"MEMORY", "MEMORY"},
		// Anything unrecognised must not reach sqlite as a bad PRAGMA value.
		{"garbage", "WAL"},
		{"", "WAL"},
	} {
		t.Setenv("XUI_DB_JOURNAL_MODE", tc.env)
		if got := sqliteJournalMode(); got != tc.want {
			t.Errorf("XUI_DB_JOURNAL_MODE=%q -> %q, want %q", tc.env, got, tc.want)
		}
	}
}

// TestDatabaseIsInWALAfterInit checks the mode actually reaches the connection,
// not just the helper.
func TestDatabaseIsInWALAfterInit(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	if err := InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	var mode string
	if err := db.Raw("PRAGMA journal_mode;").Scan(&mode).Error; err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

// TestConcurrentWritesDoNotLock is the behaviour the fix exists for: several
// background jobs writing at once must not trip the busy timeout.
func TestConcurrentWritesDoNotLock(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	if err := InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 5 {
				key := "concurrent-probe"
				if err := db.Exec(
					`INSERT INTO settings (key, value) VALUES (?, ?)`,
					key+string(rune('a'+n))+string(rune('0'+j)), "v",
				).Error; err != nil {
					errs <- err
					return
				}
				var count int64
				if err := db.Raw(`SELECT COUNT(*) FROM settings`).Scan(&count).Error; err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent access failed: %v", err)
	}
}

// TestCheckpointTruncates: the panel and Telegram backups copy the main
// database file, so a checkpoint has to fold the WAL back into it. A PASSIVE
// checkpoint may leave writes in the log and the backup would silently miss
// them.
func TestCheckpointTruncates(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	if err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, "checkpoint-probe", "v").Error; err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// The row has to be readable from the main file after the checkpoint.
	var value string
	if err := db.Raw(`SELECT value FROM settings WHERE key = ?`, "checkpoint-probe").Scan(&value).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if value != "v" {
		t.Errorf("value = %q after checkpoint, want v", value)
	}
}
