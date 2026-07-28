package service

import (
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"gorm.io/gorm"
)

// Short-lived tombstone of just-deleted client emails so that a node snapshot
// arriving between delete and node-side processing doesn't resurrect them.
var (
	recentlyDeletedMu sync.Mutex
	recentlyDeleted   = map[string]time.Time{}
)

const deleteTombstoneTTL = 90 * time.Second

var (
	inboundMutationLocksMu sync.Mutex
	inboundMutationLocks   = map[int]*sync.Mutex{}
)

func lockInbound(inboundId int) *sync.Mutex {
	inboundMutationLocksMu.Lock()
	defer inboundMutationLocksMu.Unlock()
	m, ok := inboundMutationLocks[inboundId]
	if !ok {
		m = &sync.Mutex{}
		inboundMutationLocks[inboundId] = m
	}
	m.Lock()
	return m
}

// compactOrphans drops clients that were deleted through the panel but are still
// carried in an inbound's JSON — a node snapshot or a concurrent write can hand
// back a list that predates the delete, and re-saving it would resurrect them.
//
// It removes a client only when its email is BOTH missing from client_records
// and tombstoned by a recent delete. A missing record on its own is not proof of
// a delete: client_records is a mirror of the inbound JSON, not its source of
// truth, and a JSON client legitimately outlives its mirror after a partial
// backup restore, a hand-edited inbound, or an import. Treating those as
// deletions turned a single client add into a silent mass delete of every
// unmirrored account on that inbound. Callers write the compacted list back and
// then SyncInbound, which adopts the survivors and heals the mirror.
func compactOrphans(db *gorm.DB, clients []any) []any {
	if len(clients) == 0 {
		return clients
	}
	// Nothing was deleted recently, so nothing here can be a leftover.
	if !anyTombstones() {
		return clients
	}
	emails := make([]string, 0, len(clients))
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if e, _ := cm["email"].(string); e != "" {
			emails = append(emails, e)
		}
	}
	if len(emails) == 0 {
		return clients
	}
	existing := make(map[string]struct{}, len(emails))
	const orphanChunk = 400
	for start := 0; start < len(emails); start += orphanChunk {
		end := min(start+orphanChunk, len(emails))
		var found []string
		if err := db.Model(&model.ClientRecord{}).Where("email IN ?", emails[start:end]).Pluck("email", &found).Error; err != nil {
			logger.Warning("compactOrphans pluck:", err)
			return clients
		}
		for _, e := range found {
			existing[e] = struct{}{}
		}
	}
	if len(existing) == len(emails) {
		return clients
	}
	out := make([]any, 0, len(clients))
	var dropped []string
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			out = append(out, c)
			continue
		}
		e, _ := cm["email"].(string)
		if e == "" {
			out = append(out, c)
			continue
		}
		if _, ok := existing[e]; ok {
			out = append(out, c)
			continue
		}
		// Unmirrored. Only a recent delete justifies removing it.
		if isClientEmailTombstoned(e) {
			dropped = append(dropped, e)
			continue
		}
		out = append(out, c)
	}
	if len(dropped) > 0 {
		logger.Debug("compactOrphans dropped just-deleted clients:", dropped)
	}
	return out
}

// clearTombstones drops every recorded delete. Tests use it so one case's
// tombstones cannot leak into the next.
func clearTombstones() {
	recentlyDeletedMu.Lock()
	defer recentlyDeletedMu.Unlock()
	clear(recentlyDeleted)
}

// anyTombstones reports whether any client was deleted recently enough that a
// stale write could still resurrect it.
func anyTombstones() bool {
	recentlyDeletedMu.Lock()
	defer recentlyDeletedMu.Unlock()
	cutoff := time.Now().Add(-deleteTombstoneTTL)
	for _, ts := range recentlyDeleted {
		if ts.After(cutoff) {
			return true
		}
	}
	return false
}

func tombstoneClientEmail(email string) {
	if email == "" {
		return
	}
	recentlyDeletedMu.Lock()
	defer recentlyDeletedMu.Unlock()
	recentlyDeleted[email] = time.Now()
	cutoff := time.Now().Add(-deleteTombstoneTTL)
	for e, ts := range recentlyDeleted {
		if ts.Before(cutoff) {
			delete(recentlyDeleted, e)
		}
	}
}

func tombstoneClientEmails(emails []string) {
	if len(emails) == 0 {
		return
	}
	now := time.Now()
	cutoff := now.Add(-deleteTombstoneTTL)
	recentlyDeletedMu.Lock()
	defer recentlyDeletedMu.Unlock()
	for _, email := range emails {
		if email != "" {
			recentlyDeleted[email] = now
		}
	}
	for e, ts := range recentlyDeleted {
		if ts.Before(cutoff) {
			delete(recentlyDeleted, e)
		}
	}
}

func isClientEmailTombstoned(email string) bool {
	if email == "" {
		return false
	}
	recentlyDeletedMu.Lock()
	defer recentlyDeletedMu.Unlock()
	ts, ok := recentlyDeleted[email]
	if !ok {
		return false
	}
	if time.Since(ts) > deleteTombstoneTTL {
		delete(recentlyDeleted, email)
		return false
	}
	return true
}
