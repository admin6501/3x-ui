package salesbot

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestBlockedFromBot is the gate the whole bot now leans on: a blocked customer
// must be turned away at every entry point, an admin must never be treated as
// blocked, and an unknown or unblocked chat passes. The bug it fixes let a
// blocked user do everything but buy a config.
func TestBlockedFromBot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	rows := []model.BotUser{
		{TelegramId: 100, Blocked: true},  // blocked customer
		{TelegramId: 200, Blocked: false}, // ordinary customer
		{TelegramId: 999, Blocked: true},  // blocked, but also an admin below
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed user %d: %v", rows[i].TelegramId, err)
		}
	}

	// 999 is configured as an admin: the block flag must not lock a shop owner
	// out of their own bot.
	b := &Bot{states: newStateStore(), adminIds: []int64{999}}

	cases := []struct {
		name   string
		chatId int64
		want   bool
	}{
		{"blocked customer is turned away", 100, true},
		{"unblocked customer passes", 200, false},
		{"admin is never blocked", 999, false},
		{"unknown chat (no row) passes", 4242, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := b.blockedFromBot(tc.chatId); got != tc.want {
				t.Errorf("blockedFromBot(%d) = %v, want %v", tc.chatId, got, tc.want)
			}
		})
	}
}
