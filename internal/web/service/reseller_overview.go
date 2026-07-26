package service

import (
	"sort"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// ResellerInboundSummary is one row of the reseller's inbound breakdown: what
// they were given, how much has gone through it, and how many of their clients
// sit on it.
type ResellerInboundSummary struct {
	Id       int    `json:"id"`
	Remark   string `json:"remark"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Enable   bool   `json:"enable"`
	Up       int64  `json:"up"`
	Down     int64  `json:"down"`
	Clients  int    `json:"clients"`
}

// ResellerClientSummary is a compact client row for the dashboard's list. It
// deliberately carries no credentials — the dashboard shows status, not config.
type ResellerClientSummary struct {
	Email      string `json:"email"`
	Enable     bool   `json:"enable"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	Total      int64  `json:"total"`
	ExpiryTime int64  `json:"expiryTime"`
	Online     bool   `json:"online"`
	CreatedAt  int64  `json:"createdAt"`
}

// ResellerOverview is everything the reseller dashboard renders in one call, so
// the page doesn't have to stitch together four endpoints it is only partly
// allowed to read.
type ResellerOverview struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`

	// Quota block.
	TrafficUsedBytes    int64 `json:"trafficUsedBytes"`
	TrafficQuotaGB      int64 `json:"trafficQuotaGB"`
	CurrentClients      int   `json:"currentClients"`
	ClientQuota         int   `json:"clientQuota"`
	ClientsCreatedTotal int   `json:"clientsCreatedTotal"`

	// Client buckets, counted across every inbound assigned to the reseller.
	ClientsActive   int `json:"clientsActive"`
	ClientsOnline   int `json:"clientsOnline"`
	ClientsExpiring int `json:"clientsExpiring"`
	ClientsEnded    int `json:"clientsEnded"`
	ClientsDisabled int `json:"clientsDisabled"`

	// How much of the reseller's own traffic budget each inbound accounts for.
	Inbounds []ResellerInboundSummary `json:"inbounds"`
	// The clients closest to running out — the ones worth chasing for a renewal.
	ExpiringSoon []ResellerClientSummary `json:"expiringSoon"`
	// Most recently created clients, newest first.
	Recent []ResellerClientSummary `json:"recent"`
}

const (
	resellerRecentClients  = 8
	resellerExpiringWindow = 7 * 24 * time.Hour
)

// GetResellerOverview builds the dashboard payload for one reseller. A
// non-reseller (or a reseller with no inbounds) gets a well-formed, empty
// overview rather than an error, so the page renders either way.
func (s *AdminService) GetResellerOverview(u *model.User, inboundSvc *InboundService) ResellerOverview {
	out := ResellerOverview{
		Inbounds:     []ResellerInboundSummary{},
		ExpiringSoon: []ResellerClientSummary{},
		Recent:       []ResellerClientSummary{},
	}
	if u == nil {
		return out
	}
	out.Username = u.Username
	out.Role = u.Role
	out.Disabled = u.Disabled
	if u.Role != model.RoleReseller {
		return out
	}

	stats := s.GetResellerStats(u)
	out.TrafficUsedBytes = stats.TrafficUsedBytes
	out.TrafficQuotaGB = stats.TrafficQuotaGB
	out.CurrentClients = stats.CurrentClients
	out.ClientQuota = stats.ClientQuota
	out.ClientsCreatedTotal = stats.ClientsCreatedTotal

	ids := AllowedInboundIDs(u)
	if len(ids) == 0 {
		return out
	}

	db := database.GetDB()

	var inbounds []model.Inbound
	if err := db.Model(&model.Inbound{}).Where("id IN ?", ids).Order("id ASC").Find(&inbounds).Error; err != nil {
		return out
	}

	// client_inbounds is the authoritative link between a client and the
	// inbounds it is attached to, so both the per-inbound counts and the client
	// list come from it rather than from parsing every inbound's settings JSON.
	type link struct {
		ClientId  int
		InboundId int
	}
	var links []link
	_ = db.Model(&model.ClientInbound{}).
		Select("client_id, inbound_id").
		Where("inbound_id IN ?", ids).
		Scan(&links).Error

	clientsPerInbound := make(map[int]int, len(inbounds))
	clientIDSet := make(map[int]struct{}, len(links))
	for _, l := range links {
		clientsPerInbound[l.InboundId]++
		clientIDSet[l.ClientId] = struct{}{}
	}

	for _, ib := range inbounds {
		out.Inbounds = append(out.Inbounds, ResellerInboundSummary{
			Id:       ib.Id,
			Remark:   ib.Remark,
			Protocol: string(ib.Protocol),
			Port:     ib.Port,
			Enable:   ib.Enable,
			Up:       ib.Up,
			Down:     ib.Down,
			Clients:  clientsPerInbound[ib.Id],
		})
	}

	if len(clientIDSet) == 0 {
		return out
	}
	clientIDs := make([]int, 0, len(clientIDSet))
	for id := range clientIDSet {
		clientIDs = append(clientIDs, id)
	}

	var records []model.ClientRecord
	for _, batch := range chunkInts(clientIDs, sqlInChunk) {
		var page []model.ClientRecord
		if err := db.Model(&model.ClientRecord{}).Where("id IN ?", batch).Find(&page).Error; err != nil {
			return out
		}
		records = append(records, page...)
	}
	if len(records) == 0 {
		return out
	}

	emails := make([]string, 0, len(records))
	for _, r := range records {
		if r.Email != "" {
			emails = append(emails, r.Email)
		}
	}
	trafficByEmail := make(map[string]xray.ClientTraffic, len(emails))
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var rows []xray.ClientTraffic
		if err := db.Model(xray.ClientTraffic{}).Where("email IN ?", batch).Find(&rows).Error; err != nil {
			break
		}
		for _, row := range rows {
			trafficByEmail[strings.ToLower(row.Email)] = row
		}
	}

	onlineSet := map[string]struct{}{}
	if inboundSvc != nil {
		for _, email := range inboundSvc.GetOnlineClients() {
			onlineSet[strings.ToLower(email)] = struct{}{}
		}
	}

	now := time.Now()
	nowMs := now.UnixMilli()
	expiringCutoff := now.Add(resellerExpiringWindow).UnixMilli()

	summaries := make([]ResellerClientSummary, 0, len(records))
	for _, r := range records {
		key := strings.ToLower(r.Email)
		traffic := trafficByEmail[key]
		summary := ResellerClientSummary{
			Email:      r.Email,
			Enable:     r.Enable,
			Up:         traffic.Up,
			Down:       traffic.Down,
			Total:      traffic.Total,
			ExpiryTime: traffic.ExpiryTime,
			CreatedAt:  r.CreatedAt,
		}
		if summary.Total == 0 {
			summary.Total = r.TotalGB
		}
		if summary.ExpiryTime == 0 {
			summary.ExpiryTime = r.ExpiryTime
		}
		_, summary.Online = onlineSet[key]

		exhausted := summary.Total > 0 && summary.Up+summary.Down >= summary.Total
		expired := summary.ExpiryTime > 0 && summary.ExpiryTime <= nowMs
		switch {
		case exhausted || expired:
			out.ClientsEnded++
		case !summary.Enable:
			out.ClientsDisabled++
		default:
			out.ClientsActive++
			if summary.Online {
				out.ClientsOnline++
			}
			if summary.ExpiryTime > 0 && summary.ExpiryTime <= expiringCutoff {
				out.ClientsExpiring++
			}
		}
		summaries = append(summaries, summary)
	}

	// Renewal candidates: still running, with an expiry inside the window,
	// soonest first.
	expiring := make([]ResellerClientSummary, 0, len(summaries))
	for _, c := range summaries {
		if !c.Enable || c.ExpiryTime <= 0 || c.ExpiryTime <= nowMs || c.ExpiryTime > expiringCutoff {
			continue
		}
		expiring = append(expiring, c)
	}
	sort.Slice(expiring, func(i, j int) bool { return expiring[i].ExpiryTime < expiring[j].ExpiryTime })
	if len(expiring) > resellerRecentClients {
		expiring = expiring[:resellerRecentClients]
	}
	out.ExpiringSoon = expiring

	recent := make([]ResellerClientSummary, len(summaries))
	copy(recent, summaries)
	sort.Slice(recent, func(i, j int) bool { return recent[i].CreatedAt > recent[j].CreatedAt })
	if len(recent) > resellerRecentClients {
		recent = recent[:resellerRecentClients]
	}
	out.Recent = recent

	return out
}
