package model

// ClientActivity is one observed connection by an inbound client: who (email),
// from where (source IP), to what (destination host + network), and when.
//
// Rows are written by the activity job from the Xray access log, only while the
// clientActivityEnable setting is on. The natural key (email, ip, dest, network,
// timestamp) is unique so re-reading the log after a panel restart cannot
// double-count the same connection.
//
// This is sensitive data — it is the browsing record of people using an
// anti-censorship proxy — so it is gated to settings.manage, never shown to
// resellers, and wipeable in full from the Client Activity page.
type ClientActivity struct {
	Id        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Email     string `json:"email" gorm:"index:idx_client_activity_email;index:idx_client_activity_dedup,unique,priority:1"`
	IP        string `json:"ip" gorm:"column:ip;index:idx_client_activity_dedup,unique,priority:2"`
	Dest      string `json:"dest" gorm:"index:idx_client_activity_dedup,unique,priority:3"`
	Network   string `json:"network" gorm:"index:idx_client_activity_dedup,unique,priority:4"`
	Timestamp int64  `json:"timestamp" gorm:"index:idx_client_activity_ts;index:idx_client_activity_dedup,unique,priority:5"`
}

func (ClientActivity) TableName() string { return "client_activities" }

// IPGeo caches the network operator behind a source IP so the activity page can
// show an ISP/country without re-querying for every row. Enrichment is
// best-effort: an unresolved IP simply has empty fields and is retried later.
type IPGeo struct {
	IP          string `json:"ip" gorm:"column:ip;primaryKey"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode" gorm:"column:country_code"`
	ISP         string `json:"isp" gorm:"column:isp"`
	Org         string `json:"org"`
	ASN         string `json:"asn" gorm:"column:asn"`
	UpdatedAt   int64  `json:"updatedAt"`
}

func (IPGeo) TableName() string { return "ip_geo" }
