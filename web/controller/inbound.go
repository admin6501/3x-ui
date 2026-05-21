package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"github.com/mhsanaei/3x-ui/v2/web/websocket"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"github.com/gin-gonic/gin"
)

// InboundController handles HTTP requests related to Xray inbounds management.
type InboundController struct {
	inboundService service.InboundService
	xrayService    service.XrayService
	adminService   service.AdminService
}

// NewInboundController creates a new InboundController and sets up its routes.
func NewInboundController(g *gin.RouterGroup) *InboundController {
	a := &InboundController{}
	a.initRouter(g)
	return a
}

// broadcastInboundsUpdateClientLimit is the threshold past which we skip the
// full-list push over WebSocket and signal the frontend to re-fetch via REST.
// Mirrors the same heuristic used by the periodic traffic job.
const broadcastInboundsUpdateClientLimit = 5000

// broadcastInboundsUpdate fetches and broadcasts the inbound list for userId.
// At scale (10k+ clients) the marshaled JSON exceeds the WS payload ceiling,
// so we send an invalidate signal instead — frontend re-fetches via REST.
// Skipped entirely when no WebSocket clients are connected.
func (a *InboundController) broadcastInboundsUpdate(userId int) {
	if !websocket.HasClients() {
		return
	}
	inbounds, err := a.inboundService.GetInbounds(userId)
	if err != nil {
		return
	}
	totalClients := 0
	for _, ib := range inbounds {
		totalClients += len(ib.ClientStats)
	}
	if totalClients > broadcastInboundsUpdateClientLimit {
		websocket.BroadcastInvalidate(websocket.MessageTypeInbounds)
		return
	}
	websocket.BroadcastInbounds(inbounds)
}

// initRouter initializes the routes for inbound-related operations.
func (a *InboundController) initRouter(g *gin.RouterGroup) {

	g.GET("/list", a.getInbounds)
	g.GET("/get/:id", a.scopeInboundParam, a.getInbound)
	g.GET("/getClientTraffics/:email", a.scopeClientByEmail, a.getClientTraffics)
	g.GET("/getClientTrafficsById/:id", a.scopeClientByUUID, a.getClientTrafficsById)
	// myQuota returns the fresh traffic_quota / traffic_used for the
	// CURRENT logged-in user. The inbounds page renders a quota progress
	// card from the Go template snapshot at page-load; this endpoint lets
	// it re-read the live values after the reseller mutates traffic (add
	// client / delete client) without a full page refresh.
	g.GET("/myQuota", a.getMyQuota)

	g.POST("/add", a.scopeRejectResellerCreate, a.addInbound)
	g.POST("/del/:id", a.scopeRejectResellerInboundEdit, a.scopeInboundParam, a.delInbound)
	g.POST("/update/:id", a.scopeRejectResellerInboundEdit, a.scopeInboundParam, a.updateInbound)
	g.POST("/setEnable/:id", a.scopeRejectResellerInboundEdit, a.scopeInboundParam, a.setInboundEnable)
	g.POST("/clientIps/:email", a.scopeClientByEmail, a.getClientIps)
	g.POST("/clearClientIps/:email", a.scopeClientByEmail, a.clearClientIps)
	g.POST("/addClient", a.addInboundClient)
	g.POST("/:id/copyClients", a.scopeInboundParam, a.copyInboundClients)
	g.POST("/:id/delClient/:clientId", a.scopeInboundParam, a.delInboundClient)
	g.POST("/updateClient/:clientId", a.updateInboundClient)
	g.POST("/:id/resetClientTraffic/:email", a.scopeInboundParam, a.resetClientTraffic)
	g.POST("/resetAllTraffics", a.scopeRejectReseller, a.resetAllTraffics)
	g.POST("/resetAllClientTraffics/:id", a.scopeInboundParam, a.resetAllClientTraffics)
	g.POST("/delDepletedClients/:id", a.scopeInboundParam, a.delDepletedClients)
	g.POST("/import", a.scopeRejectResellerCreate, a.importInbound)
	g.POST("/onlines", a.onlines)
	g.POST("/lastOnline", a.lastOnline)
	g.POST("/updateClientTraffic/:email", a.scopeClientByEmail, a.updateClientTraffic)
	g.POST("/:id/delClientByEmail/:email", a.scopeInboundParam, a.delInboundClientByEmail)
}

// scopeInboundParam ensures the URL :id is in the reseller's allowed set.
// No-op for super_admin / manager / readonly.
func (a *InboundController) scopeInboundParam(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		// Not a valid id — let the downstream handler 400; the scope
		// guard has nothing to enforce.
		c.Next()
		return
	}
	if !enforceInboundScope(c, id) {
		return
	}
	c.Next()
}

// scopeClientByEmail enforces reseller scope on endpoints whose only URL
// parameter is a client email. Wrapper around enforceInboundScopeByEmail.
// No-op for super_admin / manager / readonly.
func (a *InboundController) scopeClientByEmail(c *gin.Context) {
	email := c.Param("email")
	if !enforceInboundScopeByEmail(c, email) {
		return
	}
	c.Next()
}

// scopeClientByUUID enforces reseller scope on endpoints whose URL
// parameter is a client UUID/password (not an inbound id). Wrapper around
// enforceInboundScopeByClientUUID. No-op for non-resellers.
func (a *InboundController) scopeClientByUUID(c *gin.Context) {
	uuid := c.Param("id")
	if !enforceInboundScopeByClientUUID(c, uuid) {
		return
	}
	c.Next()
}

// scopeRejectResellerCreate blocks resellers from creating brand-new
// inbounds (`add`, `import`) — those would land outside their scope and
// cannot be self-granted, so they're disallowed altogether.
func (a *InboundController) scopeRejectResellerCreate(c *gin.Context) {
	if u := session.GetLoginUser(c); u != nil && u.Role == model.RoleReseller {
		jsonMsg(c, "forbidden", fmt.Errorf("resellers cannot create new inbounds"))
		c.Abort()
		return
	}
	c.Next()
}

// scopeRejectReseller blocks resellers from panel-wide actions that fan
// out across every inbound (e.g. resetAllTraffics).
func (a *InboundController) scopeRejectReseller(c *gin.Context) {
	if u := session.GetLoginUser(c); u != nil && u.Role == model.RoleReseller {
		jsonMsg(c, "forbidden", fmt.Errorf("resellers cannot trigger panel-wide actions"))
		c.Abort()
		return
	}
	c.Next()
}

// scopeRejectResellerInboundEdit blocks resellers from delete / update /
// enable-disable operations on the inbound *itself*. Resellers may only
// manage clients inside an inbound that was assigned to them — flipping the
// inbound off, editing its protocol/port, or deleting it is a panel-level
// action reserved for super_admin / manager. Without this guard the scope
// check would happily let a reseller toggle "their" inbound off, which would
// silently kill traffic for every other reseller sharing that inbound (or
// for clients owned by the super admin).
func (a *InboundController) scopeRejectResellerInboundEdit(c *gin.Context) {
	if u := session.GetLoginUser(c); u != nil && u.Role == model.RoleReseller {
		jsonMsg(c, "forbidden", fmt.Errorf("resellers can only manage clients, not the inbound itself"))
		c.Abort()
		return
	}
	c.Next()
}

type CopyInboundClientsRequest struct {
	SourceInboundID int      `form:"sourceInboundId" json:"sourceInboundId"`
	ClientEmails    []string `form:"clientEmails" json:"clientEmails"`
	Flow            string   `form:"flow" json:"flow"`
}

// getInbounds retrieves the list of inbounds visible to the logged-in
// admin: super_admin / manager / readonly see everything; reseller sees
// only the inbounds in their AllowedInbounds scope.
func (a *InboundController) getInbounds(c *gin.Context) {
	inbounds, err := a.inboundService.GetAllInbounds()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	inbounds = filterInboundsForRole(c, inbounds)
	jsonObj(c, inbounds, nil)
}

// getInbound retrieves a specific inbound by its ID.
func (a *InboundController) getInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "get"), err)
		return
	}
	inbound, err := a.inboundService.GetInbound(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	jsonObj(c, inbound, nil)
}

// getClientTraffics retrieves client traffic information by email.
func (a *InboundController) getClientTraffics(c *gin.Context) {
	email := c.Param("email")
	clientTraffics, err := a.inboundService.GetClientTrafficByEmail(email)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.trafficGetError"), err)
		return
	}
	jsonObj(c, clientTraffics, nil)
}

// getClientTrafficsById retrieves client traffic information by inbound ID.
func (a *InboundController) getClientTrafficsById(c *gin.Context) {
	id := c.Param("id")
	clientTraffics, err := a.inboundService.GetClientTrafficByID(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.trafficGetError"), err)
		return
	}
	jsonObj(c, clientTraffics, nil)
}

// getMyQuota returns the fresh traffic_quota / traffic_used for the
// currently-authenticated user. Used by the inbounds page to update
// the reseller quota card in-place after a client add/update/delete.
// Returns zeros (no error) for unauthenticated requests and for
// non-reseller roles whose values are meaningless in the UI; that keeps
// the response shape stable so the JS doesn't need a special-case branch.
func (a *InboundController) getMyQuota(c *gin.Context) {
	type myQuotaResp struct {
		Role         string `json:"role"`
		TrafficQuota int64  `json:"trafficQuota"`
		TrafficUsed  int64  `json:"trafficUsed"`
	}
	u := session.GetLoginUser(c)
	if u == nil {
		jsonObj(c, myQuotaResp{}, nil)
		return
	}
	// Re-read from the DB so we don't return a stale session snapshot
	// (the session's TrafficUsed is updated in-memory after each
	// AccumulateUsage call but only this DB read survives a refresh).
	fresh, err := a.adminService.GetUserByID(u.Id)
	if err != nil || fresh == nil {
		// Fall back to whatever the session has — better than a 500.
		jsonObj(c, myQuotaResp{
			Role:         u.Role,
			TrafficQuota: u.TrafficQuota,
			TrafficUsed:  u.TrafficUsed,
		}, nil)
		return
	}
	jsonObj(c, myQuotaResp{
		Role:         fresh.Role,
		TrafficQuota: fresh.TrafficQuota,
		TrafficUsed:  fresh.TrafficUsed,
	}, nil)
}

// addInbound creates a new inbound configuration.
func (a *InboundController) addInbound(c *gin.Context) {
	inbound := &model.Inbound{}
	err := c.ShouldBind(inbound)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundCreateSuccess"), err)
		return
	}
	user := session.GetLoginUser(c)
	inbound.UserId = user.Id
	if inbound.Listen == "" || inbound.Listen == "0.0.0.0" || inbound.Listen == "::" || inbound.Listen == "::0" {
		inbound.Tag = fmt.Sprintf("inbound-%v", inbound.Port)
	} else {
		inbound.Tag = fmt.Sprintf("inbound-%v:%v", inbound.Listen, inbound.Port)
	}

	inbound, needRestart, err := a.inboundService.AddInbound(inbound)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundCreateSuccess"), inbound, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	a.broadcastInboundsUpdate(user.Id)
}

// computeClientRefund returns how many bytes should be refunded to the
// reseller who owns the inbound when this client is deleted. The refund is
// the *unused* portion of the client's quota:
//
//	refund = max(0, client.TotalGB - (consumed.Up + consumed.Down))
//
// A totalGB of 0 (unlimited) has no allocated cap to refund so it returns 0.
// `consumed` may be nil — that case is treated as zero usage (fresh client).
func computeClientRefund(client model.Client, consumed *xray.ClientTraffic) int64 {
	if client.TotalGB <= 0 {
		return 0
	}
	var used int64
	if consumed != nil {
		used = consumed.Up + consumed.Down
	}
	refund := client.TotalGB - used
	if refund < 0 {
		return 0
	}
	return refund
}

// refundClientToOwner looks up the inbound's owner and refunds `refund`
// bytes to their reseller quota. No-op when the owner is not a reseller,
// when refund is non-positive, or when the lookup fails (we never fail
// the parent delete on a refund error — the client is already gone).
func (a *InboundController) refundClientToOwner(inbound *model.Inbound, refund int64) {
	if inbound == nil || refund <= 0 || inbound.UserId <= 0 {
		return
	}
	owner, err := a.adminService.GetUserByID(inbound.UserId)
	if err != nil || owner == nil {
		return
	}
	_ = a.adminService.RefundUsage(owner, refund)
}

// delInbound deletes an inbound configuration by its ID.
func (a *InboundController) delInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundDeleteSuccess"), err)
		return
	}
	// Capture the inbound + its clients BEFORE deletion so we can refund
	// the owning reseller the unused portion of each client's quota.
	// Failures here are non-fatal — the delete still proceeds; the worst
	// case is the reseller doesn't get a refund.
	var totalRefund int64
	var owningInbound *model.Inbound
	if ib, ferr := a.inboundService.GetInbound(id); ferr == nil && ib != nil {
		owningInbound = ib
		if clients, cerr := a.inboundService.GetClients(ib); cerr == nil {
			for _, cl := range clients {
				if cl.TotalGB <= 0 {
					continue
				}
				t, _ := a.inboundService.GetClientTrafficByEmail(cl.Email)
				totalRefund += computeClientRefund(cl, t)
			}
		}
	}
	needRestart, err := a.inboundService.DelInbound(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.refundClientToOwner(owningInbound, totalRefund)
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundDeleteSuccess"), id, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	user := session.GetLoginUser(c)
	a.broadcastInboundsUpdate(user.Id)
}

// updateInbound updates an existing inbound configuration.
func (a *InboundController) updateInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}
	inbound := &model.Inbound{
		Id: id,
	}
	err = c.ShouldBind(inbound)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}
	// Always trust the URL :id, never the body. Otherwise a reseller
	// with scope on inbound A could PUT body{id: B} and update inbound B
	// via the A-scoped URL.
	inbound.Id = id
	inbound, needRestart, err := a.inboundService.UpdateInbound(inbound)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), inbound, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	user := session.GetLoginUser(c)
	a.broadcastInboundsUpdate(user.Id)
}

// setInboundEnable flips only the enable flag of an inbound. This is a
// dedicated endpoint because the regular update path serialises the entire
// settings JSON (every client) — far too heavy for an interactive switch
// on inbounds with thousands of clients. Frontend optimistically updates
// the UI; we just persist + sync xray + nudge other open admin sessions.
func (a *InboundController) setInboundEnable(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}
	type form struct {
		Enable bool `json:"enable" form:"enable"`
	}
	var f form
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	needRestart, err := a.inboundService.SetInboundEnable(id, f.Enable)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	// Cross-admin sync: lightweight invalidate signal (a few hundred bytes)
	// instead of fetching + serialising the whole inbound list. Other open
	// sessions re-fetch via REST. The toggling admin's own UI already
	// updated optimistically.
	websocket.BroadcastInvalidate(websocket.MessageTypeInbounds)
}

// getClientIps retrieves the IP addresses associated with a client by email.
func (a *InboundController) getClientIps(c *gin.Context) {
	email := c.Param("email")

	ips, err := a.inboundService.GetInboundClientIps(email)
	if err != nil || ips == "" {
		jsonObj(c, "No IP Record", nil)
		return
	}

	// Prefer returning a normalized string list for consistent UI rendering
	type ipWithTimestamp struct {
		IP        string `json:"ip"`
		Timestamp int64  `json:"timestamp"`
	}

	var ipsWithTime []ipWithTimestamp
	if err := json.Unmarshal([]byte(ips), &ipsWithTime); err == nil && len(ipsWithTime) > 0 {
		formatted := make([]string, 0, len(ipsWithTime))
		for _, item := range ipsWithTime {
			if item.IP == "" {
				continue
			}
			if item.Timestamp > 0 {
				ts := time.Unix(item.Timestamp, 0).Local().Format("2006-01-02 15:04:05")
				formatted = append(formatted, fmt.Sprintf("%s (%s)", item.IP, ts))
				continue
			}
			formatted = append(formatted, item.IP)
		}
		jsonObj(c, formatted, nil)
		return
	}

	var oldIps []string
	if err := json.Unmarshal([]byte(ips), &oldIps); err == nil && len(oldIps) > 0 {
		jsonObj(c, oldIps, nil)
		return
	}

	// If parsing fails, return as string
	jsonObj(c, ips, nil)
}

// clearClientIps clears the IP addresses for a client by email.
func (a *InboundController) clearClientIps(c *gin.Context) {
	email := c.Param("email")

	err := a.inboundService.ClearClientIps(email)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.updateSuccess"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.logCleanSuccess"), nil)
}

// addInboundClient adds a new client to an existing inbound.
func (a *InboundController) addInboundClient(c *gin.Context) {
	data := &model.Inbound{}
	err := c.ShouldBind(data)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}

	// Reseller scope guard: prevent adding clients into inbounds the
	// reseller does not own.
	if !enforceInboundScope(c, data.Id) {
		return
	}

	// Reseller quota guard. We parse the new clients out of the payload
	// (data.Settings is JSON with a "clients" array) and sum their TotalGB
	// — for a reseller this must fit inside their remaining TrafficQuota.
	// All-or-nothing per the spec: if the sum overflows the cap, the whole
	// add call is rejected so a bulk-add never partially succeeds.
	actor := session.GetLoginUser(c)
	if actor != nil && actor.Role == model.RoleReseller {
		newClients, perr := a.inboundService.GetClients(data)
		if perr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), perr)
			return
		}
		var sum int64
		for _, cl := range newClients {
			// totalGB == 0 means *unlimited* on the client side. Only
			// allow it when the reseller themselves has an unlimited
			// quota (TrafficQuota == 0). Otherwise it would let one
			// client drain the reseller's whole cap silently.
			if cl.TotalGB <= 0 {
				if actor.TrafficQuota > 0 {
					jsonMsg(c, "forbidden",
						fmt.Errorf("resellers with a traffic quota cannot create unlimited (totalGB=0) clients; assign a non-zero limit"))
					return
				}
				continue // unlimited reseller — unlimited clients are fine
			}
			sum += cl.TotalGB
		}
		if err := a.adminService.CheckResellerQuota(actor, sum); err != nil {
			jsonMsg(c, "quota exceeded", err)
			return
		}

		needRestart, err := a.inboundService.AddInboundClient(data)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		// Only bump usage AFTER the DB write succeeded. We intentionally
		// accumulate even if needRestart is true — xray may pick the
		// client up only after the restart, but the allocation is
		// committed and the cap should reflect that.
		_ = a.adminService.AccumulateUsage(actor, sum)
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientAddSuccess"), nil)
		if needRestart {
			a.xrayService.SetToNeedRestart()
		}
		return
	}

	needRestart, err := a.inboundService.AddInboundClient(data)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientAddSuccess"), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
}

// copyInboundClients copies clients from source inbound to target inbound.
func (a *InboundController) copyInboundClients(c *gin.Context) {
	targetID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}

	req := &CopyInboundClientsRequest{}
	err = c.ShouldBind(req)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if req.SourceInboundID <= 0 {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), fmt.Errorf("invalid source inbound id"))
		return
	}

	// Reseller scope guard: copying clients implies read access on the
	// source AND write access on the target. The target is already
	// enforced via scopeInboundParam middleware on the URL :id; here we
	// additionally require the source to be inside the reseller's scope.
	if !enforceInboundScope(c, req.SourceInboundID) {
		return
	}

	// Reseller quota guard for copy-clients: every cloned client adds its
	// own totalGB to the reseller's used counter. Pre-flight the entire
	// batch so we either copy them all or none (no partial billing).
	actor := session.GetLoginUser(c)
	var copyQuotaBill int64
	if actor != nil && actor.Role == model.RoleReseller {
		srcInbound, serr := a.inboundService.GetInbound(req.SourceInboundID)
		if serr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), serr)
			return
		}
		srcClients, serr := a.inboundService.GetClients(srcInbound)
		if serr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), serr)
			return
		}
		// Build a set of selected emails for O(1) lookup. Empty selection
		// means "copy all" — match the service's behaviour.
		var emailSet map[string]struct{}
		if len(req.ClientEmails) > 0 {
			emailSet = make(map[string]struct{}, len(req.ClientEmails))
			for _, e := range req.ClientEmails {
				emailSet[e] = struct{}{}
			}
		}
		for _, sc := range srcClients {
			if emailSet != nil {
				if _, ok := emailSet[sc.Email]; !ok {
					continue
				}
			}
			if sc.TotalGB <= 0 {
				if actor.TrafficQuota > 0 {
					jsonMsg(c, "forbidden",
						fmt.Errorf("copy aborted: source contains unlimited client %q which a quota-bound reseller cannot duplicate", sc.Email))
					return
				}
				continue
			}
			copyQuotaBill += sc.TotalGB
		}
		if err := a.adminService.CheckResellerQuota(actor, copyQuotaBill); err != nil {
			jsonMsg(c, "quota exceeded", err)
			return
		}
	}

	result, needRestart, err := a.inboundService.CopyInboundClients(targetID, req.SourceInboundID, req.ClientEmails, req.Flow)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if copyQuotaBill > 0 {
		_ = a.adminService.AccumulateUsage(actor, copyQuotaBill)
	}
	jsonObj(c, result, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
}

// delInboundClient deletes a client from an inbound by inbound ID and client ID.
func (a *InboundController) delInboundClient(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}
	clientId := c.Param("clientId")

	// Capture refund-relevant state BEFORE the delete: locate the client in
	// the inbound's settings (clientId is the UUID/ID/Password depending on
	// protocol — we match against Client.ID first, then Email as a fallback
	// because some protocols use email-as-id) and read its current consumed
	// counter from client_traffics. After the delete succeeds, refund the
	// inbound owner's reseller quota by `totalGB - consumed`.
	var refund int64
	var owningInbound *model.Inbound
	if ib, ferr := a.inboundService.GetInbound(id); ferr == nil && ib != nil {
		owningInbound = ib
		if clients, cerr := a.inboundService.GetClients(ib); cerr == nil {
			for _, cl := range clients {
				if cl.ID != clientId && cl.Password != clientId && cl.Email != clientId && cl.Auth != clientId {
					continue
				}
				if cl.TotalGB > 0 {
					t, _ := a.inboundService.GetClientTrafficByEmail(cl.Email)
					refund = computeClientRefund(cl, t)
				}
				break
			}
		}
	}

	needRestart, err := a.inboundService.DelInboundClient(id, clientId)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.refundClientToOwner(owningInbound, refund)
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientDeleteSuccess"), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
}

// updateInboundClient updates a client's configuration in an inbound.
func (a *InboundController) updateInboundClient(c *gin.Context) {
	clientId := c.Param("clientId")

	inbound := &model.Inbound{}
	err := c.ShouldBind(inbound)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}

	// Reseller scope guard.
	if !enforceInboundScope(c, inbound.Id) {
		return
	}

	// Reseller quota guard. An *update* only costs quota when the totalGB
	// is INCREASED — the delta (new - old) is what gets billed. A reduction
	// or no-change is free; we never refund (no negative accumulate). The
	// "old" totalGB comes from the live DB row so resellers can't pre-edit
	// their copy of the inbound to fake a smaller delta.
	actor := session.GetLoginUser(c)
	if actor != nil && actor.Role == model.RoleReseller {
		newClients, perr := a.inboundService.GetClients(inbound)
		if perr != nil || len(newClients) == 0 {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"),
				fmt.Errorf("invalid client payload"))
			return
		}
		newTotal := newClients[0].TotalGB

		// Disallow flipping a client to unlimited unless the reseller is
		// themselves unlimited — same rule as the create path.
		if newTotal <= 0 && actor.TrafficQuota > 0 {
			jsonMsg(c, "forbidden",
				fmt.Errorf("resellers with a traffic quota cannot set client totalGB to 0 (unlimited)"))
			return
		}

		// Look up the OLD totalGB by clientId from the current DB row.
		oldInbound, oerr := a.inboundService.GetInbound(inbound.Id)
		if oerr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), oerr)
			return
		}
		oldClients, oerr := a.inboundService.GetClients(oldInbound)
		if oerr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), oerr)
			return
		}
		var oldTotal int64
		for _, oc := range oldClients {
			// clientId matches one of: ID (vless/vmess), Password (trojan),
			// Email (shadowsocks), Auth (hysteria). We compare against all
			// four — only one will ever match per protocol.
			if oc.ID == clientId || oc.Password == clientId || oc.Email == clientId || oc.Auth == clientId {
				oldTotal = oc.TotalGB
				break
			}
		}

		delta := newTotal - oldTotal
		if delta > 0 {
			if err := a.adminService.CheckResellerQuota(actor, delta); err != nil {
				jsonMsg(c, "quota exceeded", err)
				return
			}
		}

		needRestart, err := a.inboundService.UpdateInboundClient(inbound, clientId)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		if delta > 0 {
			_ = a.adminService.AccumulateUsage(actor, delta)
		}
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientUpdateSuccess"), nil)
		if needRestart {
			a.xrayService.SetToNeedRestart()
		}
		return
	}

	needRestart, err := a.inboundService.UpdateInboundClient(inbound, clientId)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientUpdateSuccess"), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
}

// resetClientTraffic resets the traffic counter for a specific client in an inbound.
func (a *InboundController) resetClientTraffic(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}
	email := c.Param("email")

	// For reseller accounting: capture the up+down BEFORE the reset, so we
	// can bill that amount against the reseller's quota. Resetting a client
	// is effectively re-allocating their previously-consumed bytes — without
	// this the reset would be a free way to extend a client indefinitely
	// (the user mentioned this is THE most important guardrail).
	var consumedBeforeReset int64
	actor := session.GetLoginUser(c)
	if actor != nil && actor.Role == model.RoleReseller {
		if t, terr := a.inboundService.GetClientTrafficByEmail(email); terr == nil && t != nil {
			consumedBeforeReset = t.Up + t.Down
		}
	}

	needRestart, err := a.inboundService.ResetClientTraffic(id, email)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if consumedBeforeReset > 0 {
		_ = a.adminService.AccumulateUsage(actor, consumedBeforeReset)
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.resetInboundClientTrafficSuccess"), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
}

// resetAllTraffics resets all traffic counters across all inbounds.
func (a *InboundController) resetAllTraffics(c *gin.Context) {
	err := a.inboundService.ResetAllTraffics()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	} else {
		a.xrayService.SetToNeedRestart()
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.resetAllTrafficSuccess"), nil)
}

// resetAllClientTraffics resets traffic counters for all clients in a specific inbound.
func (a *InboundController) resetAllClientTraffics(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}

	// Reseller accounting for bulk reset: sum up+down across every client
	// in this inbound before resetting. Same rationale as the single-client
	// reset path — re-allocating already-consumed bytes spends quota.
	var bulkBill int64
	actor := session.GetLoginUser(c)
	if actor != nil && actor.Role == model.RoleReseller {
		bulkBill = a.inboundService.SumInboundClientTraffic(id)
	}

	err = a.inboundService.ResetAllClientTraffics(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	} else {
		a.xrayService.SetToNeedRestart()
	}
	if bulkBill > 0 {
		_ = a.adminService.AccumulateUsage(actor, bulkBill)
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.resetAllClientTrafficSuccess"), nil)
}

// importInbound imports an inbound configuration from provided data.
func (a *InboundController) importInbound(c *gin.Context) {
	inbound := &model.Inbound{}
	err := json.Unmarshal([]byte(c.PostForm("data")), inbound)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	user := session.GetLoginUser(c)
	inbound.Id = 0
	inbound.UserId = user.Id
	if inbound.Listen == "" || inbound.Listen == "0.0.0.0" || inbound.Listen == "::" || inbound.Listen == "::0" {
		inbound.Tag = fmt.Sprintf("inbound-%v", inbound.Port)
	} else {
		inbound.Tag = fmt.Sprintf("inbound-%v:%v", inbound.Listen, inbound.Port)
	}

	for index := range inbound.ClientStats {
		inbound.ClientStats[index].Id = 0
		inbound.ClientStats[index].Enable = true
	}

	needRestart := false
	inbound, needRestart, err = a.inboundService.AddInbound(inbound)
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundCreateSuccess"), inbound, err)
	if err == nil && needRestart {
		a.xrayService.SetToNeedRestart()
	}
}

// delDepletedClients deletes clients in an inbound who have exhausted their traffic limits.
func (a *InboundController) delDepletedClients(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}
	err = a.inboundService.DelDepletedClients(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.delDepletedClientsSuccess"), nil)
}

// onlines retrieves the list of currently online clients.
func (a *InboundController) onlines(c *gin.Context) {
	jsonObj(c, a.inboundService.GetOnlineClients(), nil)
}

// lastOnline retrieves the last online timestamps for clients.
func (a *InboundController) lastOnline(c *gin.Context) {
	data, err := a.inboundService.GetClientsLastOnline()
	jsonObj(c, data, err)
}

// updateClientTraffic updates the traffic statistics for a client by email.
func (a *InboundController) updateClientTraffic(c *gin.Context) {
	email := c.Param("email")

	// Define the request structure for traffic update
	type TrafficUpdateRequest struct {
		Upload   int64 `json:"upload"`
		Download int64 `json:"download"`
	}

	var request TrafficUpdateRequest
	err := c.ShouldBindJSON(&request)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundUpdateSuccess"), err)
		return
	}

	err = a.inboundService.UpdateClientTrafficByEmail(email, request.Upload, request.Download)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}

	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientUpdateSuccess"), nil)
}

// delInboundClientByEmail deletes a client from an inbound by email address.
func (a *InboundController) delInboundClientByEmail(c *gin.Context) {
	inboundId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "Invalid inbound ID", err)
		return
	}

	email := c.Param("email")

	// Same refund pre-capture as delInboundClient — see comments there.
	var refund int64
	var owningInbound *model.Inbound
	if ib, ferr := a.inboundService.GetInbound(inboundId); ferr == nil && ib != nil {
		owningInbound = ib
		if clients, cerr := a.inboundService.GetClients(ib); cerr == nil {
			for _, cl := range clients {
				if cl.Email != email {
					continue
				}
				if cl.TotalGB > 0 {
					t, _ := a.inboundService.GetClientTrafficByEmail(cl.Email)
					refund = computeClientRefund(cl, t)
				}
				break
			}
		}
	}

	needRestart, err := a.inboundService.DelInboundClientByEmail(inboundId, email)
	if err != nil {
		jsonMsg(c, "Failed to delete client by email", err)
		return
	}

	a.refundClientToOwner(owningInbound, refund)
	jsonMsg(c, "Client deleted successfully", nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
}
