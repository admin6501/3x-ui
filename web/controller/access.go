// Package controller — shared RBAC helpers used across HTTP handlers.
package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"github.com/gin-gonic/gin"
)

// resellerAllowedSet returns the set of inbound IDs the currently logged-in
// reseller is allowed to access. Returns ok=false for any non-reseller role
// (super_admin / manager / readonly) — those bypass the scope filter.
func resellerAllowedSet(c *gin.Context) (set map[int]struct{}, ok bool) {
	u := session.GetLoginUser(c)
	if u == nil || u.Role != model.RoleReseller {
		return nil, false
	}
	ids := service.AllowedInboundIDs(u)
	set = make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, true
}

// currentResellerUsername returns the username of the logged-in reseller, or
// empty if the request isn't a reseller. Used to scope client ownership.
func currentResellerUsername(c *gin.Context) string {
	u := session.GetLoginUser(c)
	if u == nil || u.Role != model.RoleReseller {
		return ""
	}
	return u.Username
}

// enforceInboundScope is a guard for endpoints that act on a specific
// inbound id. For resellers it 403s if the id is outside their allowed
// set; for everyone else it's a no-op. Returns true to continue handling.
func enforceInboundScope(c *gin.Context, inboundID int) bool {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return true
	}
	if _, ok := set[inboundID]; ok {
		return true
	}
	pureJsonMsg(c, http.StatusForbidden, false, "forbidden: inbound is outside your reseller scope")
	c.Abort()
	return false
}

// filterInboundsForRole returns only the inbounds the logged-in user is
// allowed to see. Super-admin / manager / readonly see everything; reseller
// sees only inbounds in their AllowedInbounds CSV *and* the embedded
// settings.clients are filtered down to clients the reseller owns.
func filterInboundsForRole(c *gin.Context, in []*model.Inbound) []*model.Inbound {
	set, isReseller := resellerAllowedSet(c)
	if !isReseller {
		return in
	}
	username := currentResellerUsername(c)
	out := make([]*model.Inbound, 0, len(in))
	for _, ib := range in {
		if _, ok := set[ib.Id]; !ok {
			continue
		}
		clone := *ib
		filterInboundClientsToOwner(&clone, username)
		out = append(out, &clone)
	}
	return out
}

// filterInboundClientsToOwner rewrites the inbound's settings JSON so the
// embedded "clients" array only contains entries whose ownerUsername equals
// `owner`. Clients without an ownerUsername are treated as legacy (created
// before this feature) and hidden from resellers — they belong to the
// super-admin. Inbound types without a "clients" array (e.g. wireguard,
// shadowsocks single-user) are left untouched. Also filters ClientStats so
// the traffic table on the frontend only shows the reseller's own clients.
func filterInboundClientsToOwner(ib *model.Inbound, owner string) {
	if owner == "" || ib == nil || ib.Settings == "" {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return
	}
	clients, ok := raw["clients"].([]any)
	if !ok {
		return
	}
	ownedEmails := make(map[string]struct{}, len(clients))
	filtered := make([]any, 0, len(clients))
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		cmOwner, _ := cm["ownerUsername"].(string)
		if cmOwner == owner {
			filtered = append(filtered, cm)
			if em, _ := cm["email"].(string); em != "" {
				ownedEmails[em] = struct{}{}
			}
		}
	}
	raw["clients"] = filtered
	if buf, err := json.Marshal(raw); err == nil {
		ib.Settings = string(buf)
	}
	// Mirror the filter onto ClientStats so the per-client traffic table
	// on the inbound page doesn't leak rows from other resellers' clients.
	if len(ib.ClientStats) > 0 {
		newStats := make([]xray.ClientTraffic, 0, len(ib.ClientStats))
		for _, cs := range ib.ClientStats {
			if _, ok := ownedEmails[cs.Email]; ok {
				newStats = append(newStats, cs)
			}
		}
		ib.ClientStats = newStats
	}
}

// enforceClientOwnership returns true if the logged-in user is allowed to
// act on the given client email inside the given inbound. Super-admin /
// manager / readonly always pass. Resellers pass only when the client's
// ownerUsername matches their own username. A 403 is written on rejection.
func enforceClientOwnership(c *gin.Context, inboundService service.InboundService, inboundID int, email string) bool {
	owner := currentResellerUsername(c)
	if owner == "" {
		return true // non-reseller
	}
	ib, err := inboundService.GetInbound(inboundID)
	if err != nil || ib == nil {
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: inbound not found")
		c.Abort()
		return false
	}
	if clientOwnerFromSettings(ib.Settings, email) == owner {
		return true
	}
	pureJsonMsg(c, http.StatusForbidden, false, "forbidden: client is outside your reseller scope")
	c.Abort()
	return false
}

// enforceClientOwnershipByEmail looks up the inbound that contains `email`,
// then defers to enforceClientOwnership. Used by endpoints that take only
// the email (no inbound id in the URL).
func enforceClientOwnershipByEmail(c *gin.Context, inboundService service.InboundService, email string) bool {
	owner := currentResellerUsername(c)
	if owner == "" {
		return true
	}
	_, ib, err := inboundService.GetClientInboundByEmail(email)
	if err != nil || ib == nil {
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: client not found")
		c.Abort()
		return false
	}
	if clientOwnerFromSettings(ib.Settings, email) == owner {
		return true
	}
	pureJsonMsg(c, http.StatusForbidden, false, "forbidden: client is outside your reseller scope")
	c.Abort()
	return false
}

// clientOwnerFromSettings parses settings JSON, finds the client whose email
// matches, and returns its ownerUsername (empty if not found / no owner).
func clientOwnerFromSettings(settings, email string) string {
	if settings == "" || email == "" {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(settings), &raw); err != nil {
		return ""
	}
	clients, _ := raw["clients"].([]any)
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if e, _ := cm["email"].(string); e == email {
			owner, _ := cm["ownerUsername"].(string)
			return owner
		}
	}
	return ""
}

// clientOwnerByUUID parses settings JSON, finds the client whose id matches
// the given UUID/clientId, and returns its ownerUsername.
func clientOwnerByUUID(settings, uuid string) string {
	if settings == "" || uuid == "" {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(settings), &raw); err != nil {
		return ""
	}
	clients, _ := raw["clients"].([]any)
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := cm["id"].(string); id == uuid {
			owner, _ := cm["ownerUsername"].(string)
			return owner
		}
		if pwd, _ := cm["password"].(string); pwd == uuid {
			owner, _ := cm["ownerUsername"].(string)
			return owner
		}
	}
	return ""
}

// enforceClientOwnershipByUUID guards endpoints where the client is keyed by
// its UUID (e.g. delClient, updateClient).
func enforceClientOwnershipByUUID(c *gin.Context, inboundService service.InboundService, inboundID int, uuid string) bool {
	owner := currentResellerUsername(c)
	if owner == "" {
		return true
	}
	ib, err := inboundService.GetInbound(inboundID)
	if err != nil || ib == nil {
		pureJsonMsg(c, http.StatusForbidden, false, "forbidden: inbound not found")
		c.Abort()
		return false
	}
	if clientOwnerByUUID(ib.Settings, uuid) == owner {
		return true
	}
	pureJsonMsg(c, http.StatusForbidden, false, "forbidden: client is outside your reseller scope")
	c.Abort()
	return false
}

// enforceAllClientsOwnedInPayload checks every client in an inbound payload
// (typed in by addInboundClient or updateInboundClient body). For a reseller,
// every entry must either already be marked with their ownerUsername or be
// blank (in which case it'll be stamped). Other entries (someone else's
// clients) cause a 403 — otherwise a reseller could PUT a body that
// overwrites another reseller's client.
func enforceAllClientsOwnedInPayload(c *gin.Context, ib *model.Inbound) bool {
	owner := currentResellerUsername(c)
	if owner == "" || ib == nil {
		return true
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return true
	}
	clients, ok := raw["clients"].([]any)
	if !ok {
		return true
	}
	for _, c2 := range clients {
		cm, ok := c2.(map[string]any)
		if !ok {
			continue
		}
		existing, _ := cm["ownerUsername"].(string)
		if existing != "" && existing != owner {
			pureJsonMsg(c, http.StatusForbidden, false, "forbidden: payload includes clients owned by another reseller")
			c.Abort()
			return false
		}
	}
	return true
}

// filterEmailsByOwner returns the subset of `emails` whose owning inbound
// settings carry ownerUsername == owner. Used by /onlines and /lastOnline to
// scope the visible client list down to a reseller's own clients.
func filterEmailsByOwner(inboundService service.InboundService, emails []string, owner string) []string {
	if owner == "" {
		return emails
	}
	out := make([]string, 0, len(emails))
	for _, e := range emails {
		_, ib, err := inboundService.GetClientInboundByEmail(e)
		if err != nil || ib == nil {
			continue
		}
		if clientOwnerFromSettings(ib.Settings, e) == owner {
			out = append(out, e)
		}
	}
	return out
}

// bulkAssignOwnerInSettings rewrites the inbound's settings JSON, stamping
// ownerUsername=username on every client (or only clients without an owner
// when onlyLegacy=true). Returns the number of clients whose owner was
// changed. Caller is responsible for persisting via UpdateInbound.
func bulkAssignOwnerInSettings(ib *model.Inbound, username string, onlyLegacy bool) (int, error) {
	if ib == nil || ib.Settings == "" {
		return 0, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return 0, fmt.Errorf("parse inbound settings: %w", err)
	}
	clients, ok := raw["clients"].([]any)
	if !ok || len(clients) == 0 {
		return 0, nil
	}
	changed := 0
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		existing, _ := cm["ownerUsername"].(string)
		if onlyLegacy && existing != "" {
			continue
		}
		if existing == username {
			continue
		}
		cm["ownerUsername"] = username
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	raw["clients"] = clients
	buf, err := json.Marshal(raw)
	if err != nil {
		return 0, fmt.Errorf("marshal inbound settings: %w", err)
	}
	ib.Settings = string(buf)
	return changed, nil
}

// stampClientOwnerOnInbound parses the inbound payload sent by addClient /
// addInboundClient and forces ownerUsername=owner on every client entry
// that doesn't already have one. Returns the modified settings JSON. A
// no-op for non-reseller callers (owner==""). Idempotent: if a client
// already has ownerUsername set, the existing value is preserved (so the
// super-admin can transfer ownership later by editing the JSON directly).
func stampClientOwnerOnInbound(ib *model.Inbound, owner string) error {
	if owner == "" || ib == nil || ib.Settings == "" {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return fmt.Errorf("parse inbound settings: %w", err)
	}
	clients, ok := raw["clients"].([]any)
	if !ok {
		return nil
	}
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if existing, _ := cm["ownerUsername"].(string); existing == "" {
			cm["ownerUsername"] = owner
		}
	}
	raw["clients"] = clients
	buf, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal inbound settings: %w", err)
	}
	ib.Settings = string(buf)
	return nil
}

// preserveExistingOwners merges ownerUsername from the *current* DB record
// into the incoming inbound payload for any client whose payload entry is
// missing it. This protects against frontends (the JS Client model) that
// don't round-trip the ownerUsername field — without this merge, a
// super-admin editing a reseller's client would silently clear the owner
// stamp on save. Looks up each client by id/password match.
func preserveExistingOwners(inboundService service.InboundService, ib *model.Inbound) error {
	if ib == nil || ib.Settings == "" {
		return nil
	}
	existing, err := inboundService.GetInbound(ib.Id)
	if err != nil || existing == nil || existing.Settings == "" {
		return nil
	}
	var existingRaw map[string]any
	if err := json.Unmarshal([]byte(existing.Settings), &existingRaw); err != nil {
		return nil
	}
	existingClients, _ := existingRaw["clients"].([]any)
	// Build lookup: identifier -> owner. Identifier is uuid (VLESS/VMess)
	// or password (Trojan/Shadowsocks) — both are unique within an inbound.
	ownerLookup := make(map[string]string, len(existingClients))
	for _, c := range existingClients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		owner, _ := cm["ownerUsername"].(string)
		if owner == "" {
			continue
		}
		if id, _ := cm["id"].(string); id != "" {
			ownerLookup[id] = owner
		}
		if pw, _ := cm["password"].(string); pw != "" {
			ownerLookup[pw] = owner
		}
		if em, _ := cm["email"].(string); em != "" {
			ownerLookup["email:"+em] = owner
		}
	}
	if len(ownerLookup) == 0 {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return fmt.Errorf("parse incoming settings: %w", err)
	}
	clients, _ := raw["clients"].([]any)
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if existingOwner, _ := cm["ownerUsername"].(string); existingOwner != "" {
			continue
		}
		// Probe by id, then password, then email.
		if id, _ := cm["id"].(string); id != "" {
			if o, ok := ownerLookup[id]; ok {
				cm["ownerUsername"] = o
				continue
			}
		}
		if pw, _ := cm["password"].(string); pw != "" {
			if o, ok := ownerLookup[pw]; ok {
				cm["ownerUsername"] = o
				continue
			}
		}
		if em, _ := cm["email"].(string); em != "" {
			if o, ok := ownerLookup["email:"+em]; ok {
				cm["ownerUsername"] = o
				continue
			}
		}
	}
	raw["clients"] = clients
	buf, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal merged settings: %w", err)
	}
	ib.Settings = string(buf)
	return nil
}
