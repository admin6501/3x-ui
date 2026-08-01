package service

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// activityRowCap bounds the table so a busy server cannot fill the disk.
	// When exceeded the oldest rows are pruned; the operator can also wipe
	// everything from the page.
	activityRowCap = 200000
	// activityPruneChunk is how many oldest rows a single over-cap prune drops,
	// keeping the delete cheap while still trending back under the cap.
	activityPruneChunk = 20000
	// ipGeoTTL is how long a cached ISP/country lookup is trusted before the
	// enricher refreshes it.
	ipGeoTTL = 30 * 24 * time.Hour
	// ipGeoBatch caps how many unresolved IPs one enrichment pass looks up, so a
	// backlog is worked down over several ticks instead of one large burst.
	ipGeoBatch = 50
)

// ClientActivityService records and reads per-client activity: the IPs a client
// connects from, the network operator behind each IP, and the destinations they
// reach. It is dormant unless clientActivityEnable is on.
//
// The data is the browsing record of people using an anti-censorship proxy, so
// callers gate it to settings.manage and it is wipeable in full.
type ClientActivityService struct {
	settingService SettingService
	inboundService InboundService
}

// activityCursor tracks how far into the current access log the collector has
// read. Kept in memory: on restart it resets to 0 and re-reads the current
// (periodically truncated) log, and the unique dedup index drops the repeats.
var (
	activityCursorMu sync.Mutex
	activityOffset   int64
)

// Collect appends new access-log lines to the activity table. No-op when
// tracking is disabled, when the access log is off, or when there is nothing
// new to read.
func (s *ClientActivityService) Collect() {
	enabled, err := s.settingService.GetClientActivityEnable()
	if err != nil || !enabled {
		return
	}
	path, err := xray.GetAccessLogPath()
	if err != nil || path == "" || path == "none" {
		return
	}

	activityCursorMu.Lock()
	defer activityCursorMu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return
	}
	// The access log is truncated periodically (and on rotation); if it shrank
	// below our cursor, start over from the top of the new content.
	if info.Size() < activityOffset {
		activityOffset = 0
	}
	if info.Size() == activityOffset {
		return
	}

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	if _, err := file.Seek(activityOffset, io.SeekStart); err != nil {
		return
	}

	var rows []model.ClientActivity
	reader := bufio.NewReader(file)
	var consumed int64
	for {
		line, err := reader.ReadString('\n')
		consumed += int64(len(line))
		if row, ok := parseAccessLogLine(line); ok {
			rows = append(rows, row)
		}
		if err != nil {
			// A final line without a trailing newline is incomplete — leave the
			// cursor before it so the rest is read once it is flushed.
			if err == io.EOF && !strings.HasSuffix(line, "\n") {
				consumed -= int64(len(line))
			}
			break
		}
	}
	activityOffset += consumed

	if len(rows) == 0 {
		return
	}
	db := database.GetDB()
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(rows, 500).Error; err != nil {
		logger.Warning("client activity: insert failed:", err)
		return
	}
	s.pruneOverCap(db)
}

// parseAccessLogLine extracts one activity row from an Xray access log line.
// The format is: "<date> <time> from <ip>:<port> accepted <net>:<host>:<port>
// [inbound -> outbound] email: <email>". Lines without an email (no client) or
// api traffic are skipped.
func parseAccessLogLine(line string) (model.ClientActivity, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "api -> api") {
		return model.ClientActivity{}, false
	}
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return model.ClientActivity{}, false
	}
	var row model.ClientActivity
	for i, part := range parts {
		switch {
		case i == 0 && len(parts) > 1:
			if ts, err := time.ParseInLocation("2006/01/02 15:04:05.999999", parts[0]+" "+parts[1], time.Local); err == nil {
				row.Timestamp = ts.Unix()
			}
		case part == "from":
			if i+1 < len(parts) {
				row.IP = activityHost(strings.TrimLeft(parts[i+1], "/"))
			}
		case part == "accepted":
			if i+1 < len(parts) {
				row.Network, row.Dest = splitAcceptedTarget(strings.TrimLeft(parts[i+1], "/"))
			}
		case part == "email:":
			if i+1 < len(parts) {
				row.Email = parts[i+1]
			}
		}
	}
	if row.Email == "" || row.Dest == "" || row.IP == "" {
		return model.ClientActivity{}, false
	}
	if row.Timestamp == 0 {
		row.Timestamp = time.Now().Unix()
	}
	return row, true
}

// splitAcceptedTarget turns "tcp:example.com:443" into ("tcp", "example.com"),
// keeping the network label separate from the destination host and dropping the
// port. IPv6 literals ("tcp:[::1]:443") keep their brackets stripped.
func splitAcceptedTarget(target string) (network, dest string) {
	network = "tcp"
	if i := strings.IndexByte(target, ':'); i > 0 {
		switch target[:i] {
		case "tcp", "udp":
			network = target[:i]
			target = target[i+1:]
		}
	}
	return network, activityHost(target)
}

// activityHost strips the port from a host:port, leaving bare hosts and IPv6
// literals intact.
func activityHost(hostport string) string {
	if hostport == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	// Not host:port (a bare host, or an IPv6 literal without a port).
	return strings.Trim(hostport, "[]")
}

func (s *ClientActivityService) pruneOverCap(db *gorm.DB) {
	var count int64
	if err := db.Model(&model.ClientActivity{}).Count(&count).Error; err != nil {
		return
	}
	if count <= activityRowCap {
		return
	}
	// Delete the oldest ids in a bounded chunk so one busy tick does not lock
	// the table for a huge delete.
	var cutoffIDs []int64
	if err := db.Model(&model.ClientActivity{}).
		Order("id asc").Limit(activityPruneChunk).Pluck("id", &cutoffIDs).Error; err != nil || len(cutoffIDs) == 0 {
		return
	}
	if err := db.Where("id IN ?", cutoffIDs).Delete(&model.ClientActivity{}).Error; err != nil {
		logger.Warning("client activity: prune failed:", err)
	}
}

// EnrichIPs resolves the operator behind IPs seen in activity that have no
// fresh geo cache entry. Best-effort and bounded per pass. No-op when disabled.
//
// The lookup sends source IPs to a third-party geolocation service (ip-api.com)
// through the panel's configured outbound. That is a privacy trade-off inherent
// to answering "which operator": the panel cannot know an ASN offline. It runs
// only while tracking is enabled.
func (s *ClientActivityService) EnrichIPs() {
	enabled, err := s.settingService.GetClientActivityEnable()
	if err != nil || !enabled {
		return
	}
	db := database.GetDB()
	staleBefore := time.Now().Add(-ipGeoTTL).Unix()

	var ips []string
	if err := db.Model(&model.ClientActivity{}).
		Distinct("client_activities.ip").
		Joins("LEFT JOIN ip_geo ON ip_geo.ip = client_activities.ip").
		Where("ip_geo.ip IS NULL OR ip_geo.updated_at < ?", staleBefore).
		Limit(ipGeoBatch).
		Pluck("client_activities.ip", &ips).Error; err != nil || len(ips) == 0 {
		return
	}

	client := s.settingService.NewProxiedHTTPClient(8 * time.Second)
	for _, ip := range ips {
		geo, ok := lookupIPGeo(client, ip)
		if !ok {
			continue
		}
		geo.UpdatedAt = time.Now().Unix()
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "ip"}},
			UpdateAll: true,
		}).Create(&geo).Error; err != nil {
			logger.Warning("client activity: geo cache write failed:", err)
		}
	}
}

// lookupIPGeo queries ip-api.com for one IP. Private and unspecified addresses
// are labelled locally without a network call.
func lookupIPGeo(client *http.Client, ip string) (model.IPGeo, bool) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return model.IPGeo{}, false
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return model.IPGeo{IP: ip, Country: "Private", ISP: "Private network"}, true
	}

	endpoint := "http://ip-api.com/json/" + url.PathEscape(ip) + "?fields=status,country,countryCode,isp,org,as"
	resp, err := client.Get(endpoint)
	if err != nil {
		return model.IPGeo{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return model.IPGeo{}, false
	}
	var body struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		ISP         string `json:"isp"`
		Org         string `json:"org"`
		AS          string `json:"as"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return model.IPGeo{}, false
	}
	if body.Status != "success" {
		// Cache the miss briefly (empty fields) so a bad IP is not retried every tick.
		return model.IPGeo{IP: ip}, true
	}
	return model.IPGeo{
		IP:          ip,
		Country:     body.Country,
		CountryCode: body.CountryCode,
		ISP:         body.ISP,
		Org:         body.Org,
		ASN:         body.AS,
	}, true
}

// --- read side -------------------------------------------------------------

// ClientActivitySummary is one row of the activity list: a client, how many
// distinct IPs and destinations were seen for it, the operators behind those
// IPs, whether it is online now, and when it was last seen.
type ClientActivitySummary struct {
	Email       string   `json:"email"`
	IPCount     int      `json:"ipCount"`
	DestCount   int      `json:"destCount"`
	Operators   []string `json:"operators"`
	Countries   []string `json:"countries"`
	Online      bool     `json:"online"`
	LastSeen    int64    `json:"lastSeen"`
	RecordCount int64    `json:"recordCount"`
}

// ClientActivityIP pairs a source IP with the operator resolved for it.
type ClientActivityIP struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	ISP         string `json:"isp"`
	Org         string `json:"org"`
	ASN         string `json:"asn"`
	LastSeen    int64  `json:"lastSeen"`
	Hits        int64  `json:"hits"`
}

// ClientActivityVisit is one visited destination with how often and how
// recently it was reached.
type ClientActivityVisit struct {
	Dest     string `json:"dest"`
	Network  string `json:"network"`
	Hits     int64  `json:"hits"`
	LastSeen int64  `json:"lastSeen"`
}

// ClientActivityDetail is the full per-client view: its IPs (with operators) and
// its visited destinations, most-recent first.
type ClientActivityDetail struct {
	Email  string                `json:"email"`
	Online bool                  `json:"online"`
	IPs    []ClientActivityIP    `json:"ips"`
	Visits []ClientActivityVisit `json:"visits"`
}

// Enabled reports whether tracking is currently on, so the endpoints can tell
// the UI to show the "turn it on in settings" state instead of an empty table.
func (s *ClientActivityService) Enabled() bool {
	enabled, _ := s.settingService.GetClientActivityEnable()
	return enabled
}

// ListSummaries returns one summary per client that has any recorded activity,
// ordered by most recently seen.
func (s *ClientActivityService) ListSummaries() ([]ClientActivitySummary, error) {
	db := database.GetDB()
	type agg struct {
		Email       string
		IPCount     int
		DestCount   int
		LastSeen    int64
		RecordCount int64
	}
	var rows []agg
	if err := db.Model(&model.ClientActivity{}).
		Select("email, COUNT(DISTINCT ip) as ip_count, COUNT(DISTINCT dest) as dest_count, MAX(timestamp) as last_seen, COUNT(*) as record_count").
		Group("email").
		Order("last_seen desc").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	online := make(map[string]struct{})
	for _, e := range s.inboundService.GetOnlineClients() {
		online[e] = struct{}{}
	}

	out := make([]ClientActivitySummary, 0, len(rows))
	for _, r := range rows {
		operators, countries := s.operatorsForEmail(db, r.Email)
		_, isOnline := online[r.Email]
		out = append(out, ClientActivitySummary{
			Email:       r.Email,
			IPCount:     r.IPCount,
			DestCount:   r.DestCount,
			Operators:   operators,
			Countries:   countries,
			Online:      isOnline,
			LastSeen:    r.LastSeen,
			RecordCount: r.RecordCount,
		})
	}
	return out, nil
}

// operatorsForEmail returns the distinct ISPs and countries seen for a client,
// resolved through the geo cache.
func (s *ClientActivityService) operatorsForEmail(db *gorm.DB, email string) (operators, countries []string) {
	var geos []model.IPGeo
	db.Table("ip_geo").
		Joins("JOIN client_activities ON client_activities.ip = ip_geo.ip").
		Where("client_activities.email = ?", email).
		Distinct("ip_geo.ip, ip_geo.country, ip_geo.country_code, ip_geo.isp, ip_geo.org, ip_geo.asn, ip_geo.updated_at").
		Find(&geos)
	opSet := map[string]struct{}{}
	cSet := map[string]struct{}{}
	for _, g := range geos {
		if g.ISP != "" {
			opSet[g.ISP] = struct{}{}
		}
		if g.Country != "" {
			cSet[g.Country] = struct{}{}
		}
	}
	return sortedKeys(opSet), sortedKeys(cSet)
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Detail returns a client's IPs (with operators) and up to visitLimit of its
// most recent destinations.
func (s *ClientActivityService) Detail(email string, visitLimit int) (*ClientActivityDetail, error) {
	if visitLimit <= 0 || visitLimit > 1000 {
		visitLimit = 200
	}
	db := database.GetDB()

	var ipRows []struct {
		IP       string
		Hits     int64
		LastSeen int64
	}
	if err := db.Model(&model.ClientActivity{}).
		Select("ip, COUNT(*) as hits, MAX(timestamp) as last_seen").
		Where("email = ?", email).
		Group("ip").
		Order("last_seen desc").
		Scan(&ipRows).Error; err != nil {
		return nil, err
	}

	geoByIP := map[string]model.IPGeo{}
	if len(ipRows) > 0 {
		ips := make([]string, 0, len(ipRows))
		for _, r := range ipRows {
			ips = append(ips, r.IP)
		}
		var geos []model.IPGeo
		db.Where("ip IN ?", ips).Find(&geos)
		for _, g := range geos {
			geoByIP[g.IP] = g
		}
	}

	ips := make([]ClientActivityIP, 0, len(ipRows))
	for _, r := range ipRows {
		g := geoByIP[r.IP]
		ips = append(ips, ClientActivityIP{
			IP:          r.IP,
			Country:     g.Country,
			CountryCode: g.CountryCode,
			ISP:         g.ISP,
			Org:         g.Org,
			ASN:         g.ASN,
			LastSeen:    r.LastSeen,
			Hits:        r.Hits,
		})
	}

	var visits []ClientActivityVisit
	if err := db.Model(&model.ClientActivity{}).
		Select("dest, MAX(network) as network, COUNT(*) as hits, MAX(timestamp) as last_seen").
		Where("email = ?", email).
		Group("dest").
		Order("last_seen desc").
		Limit(visitLimit).
		Scan(&visits).Error; err != nil {
		return nil, err
	}

	isOnline := false
	for _, e := range s.inboundService.GetOnlineClients() {
		if e == email {
			isOnline = true
			break
		}
	}

	return &ClientActivityDetail{Email: email, Online: isOnline, IPs: ips, Visits: visits}, nil
}

// Clear wipes all recorded activity and the geo cache. This is the "delete the
// data" action on the page; it does not touch the setting, so collection
// resumes on the next tick if tracking is still on.
func (s *ClientActivityService) Clear() error {
	db := database.GetDB()
	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.ClientActivity{}).Error; err != nil {
		return err
	}
	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.IPGeo{}).Error; err != nil {
		return err
	}
	activityCursorMu.Lock()
	activityOffset = 0
	activityCursorMu.Unlock()
	return nil
}
