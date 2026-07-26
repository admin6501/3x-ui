package service

import (
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Header names client apps use to identify the device pulling a subscription.
// Happ, Hiddify and the other apps that implement the convention send x-hwid
// plus the descriptive trio; only the HWID itself is required.
const (
	HeaderHwid        = "x-hwid"
	HeaderDeviceOS    = "x-device-os"
	HeaderOSVersion   = "x-ver-os"
	HeaderDeviceModel = "x-device-model"
)

// maxHwidFieldLen bounds what we store from request headers. The HWID is an
// opaque app-generated id (a UUID or hash in practice); anything longer is a
// client sending junk, and we neither need nor want to persist it.
const maxHwidFieldLen = 256

// DeviceCheckResult is the outcome of a subscription fetch's device check.
type DeviceCheckResult int

const (
	// DeviceAllowed means the fetch may proceed: enforcement is off, the device
	// was already known, or it fit within the client's remaining slots.
	DeviceAllowed DeviceCheckResult = iota
	// DeviceLimitReached means the client already has as many devices as it is
	// allowed and this HWID is not one of them.
	DeviceLimitReached
	// DeviceMissingHwid means the request carried no HWID and the panel is
	// configured to require one.
	DeviceMissingHwid
)

// DeviceInfo is the identifying payload lifted from a subscription request.
type DeviceInfo struct {
	Hwid        string
	DeviceOS    string
	OSVersion   string
	DeviceModel string
	UserAgent   string
	Ip          string
}

// ClientDeviceService owns the per-client device (HWID) registry: recording
// devices as they fetch a subscription, enforcing the per-client cap, and the
// admin-facing list/clear operations.
type ClientDeviceService struct {
	settingService SettingService
}

// NormalizeDeviceInfo trims and bounds the raw header values. An empty HWID
// after normalisation means "the app did not identify itself".
func NormalizeDeviceInfo(info DeviceInfo) DeviceInfo {
	clip := func(v string) string {
		v = strings.TrimSpace(v)
		if len(v) > maxHwidFieldLen {
			v = v[:maxHwidFieldLen]
		}
		return v
	}
	info.Hwid = clip(info.Hwid)
	info.DeviceOS = clip(info.DeviceOS)
	info.OSVersion = clip(info.OSVersion)
	info.DeviceModel = clip(info.DeviceModel)
	info.UserAgent = clip(info.UserAgent)
	info.Ip = clip(info.Ip)
	return info
}

// EmailBySubID resolves the client behind a subscription id. Sub ids are unique
// per client (enforced on create/update), so at most one row matches; an
// unknown id yields an empty string rather than an error, letting the caller
// treat it as "nothing to enforce against".
func (s *ClientDeviceService) EmailBySubID(subId string) (string, error) {
	subId = strings.TrimSpace(subId)
	if subId == "" {
		return "", nil
	}
	db := database.GetDB()
	var rec model.ClientRecord
	err := db.Model(model.ClientRecord{}).Where("sub_id = ?", subId).First(&rec).Error
	switch {
	case err == nil:
		return rec.Email, nil
	case database.IsNotFound(err):
		return "", nil
	default:
		return "", err
	}
}

// Enabled reports whether device-limit enforcement is switched on panel-wide.
func (s *ClientDeviceService) Enabled() bool {
	enabled, err := s.settingService.GetHwidEnable()
	return err == nil && enabled
}

// RequireHwid reports whether a subscription fetch without an HWID header is
// refused. Only meaningful while Enabled() is true.
func (s *ClientDeviceService) RequireHwid() bool {
	forced, err := s.settingService.GetHwidForced()
	return err == nil && forced
}

// GuardManualSub reports whether the browser-facing (HTML) subscription page is
// gated too. Off by default: a browser has no HWID to send, so gating it locks
// admins and users out of the info page — turn it on to close the "open the
// link in a browser and copy the configs" bypass.
func (s *ClientDeviceService) GuardManualSub() bool {
	guard, err := s.settingService.GetHwidGuardManualSub()
	return err == nil && guard
}

// EffectiveLimit resolves how many devices a client may use: its own hwidLimit
// when set, otherwise the panel-wide default. Zero means unlimited.
func (s *ClientDeviceService) EffectiveLimit(email string) (int, error) {
	db := database.GetDB()
	var rec model.ClientRecord
	err := db.Model(model.ClientRecord{}).Where("email = ?", email).First(&rec).Error
	switch {
	case err == nil:
		if rec.HwidLimit > 0 {
			return rec.HwidLimit, nil
		}
	case database.IsNotFound(err):
		// Fall through to the panel default: an email with no client row can
		// still reach here through a stale subscription id.
	default:
		return 0, err
	}
	def, err := s.settingService.GetHwidDefaultLimit()
	if err != nil {
		return 0, err
	}
	if def < 0 {
		def = 0
	}
	return def, nil
}

// Check registers the device against the client and reports whether the
// subscription fetch may proceed.
//
// A device already on the list always passes (and has its last-seen refreshed),
// so an existing user never gets locked out by their own repeat fetches. A new
// device is admitted only while the client is under its cap; once at the cap
// the fetch is refused rather than evicting someone else's device — silently
// rotating devices would make the limit invisible to the user.
func (s *ClientDeviceService) Check(email string, info DeviceInfo) (DeviceCheckResult, error) {
	if !s.Enabled() {
		return DeviceAllowed, nil
	}
	info = NormalizeDeviceInfo(info)
	if info.Hwid == "" {
		if s.RequireHwid() {
			return DeviceMissingHwid, nil
		}
		return DeviceAllowed, nil
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return DeviceAllowed, nil
	}

	limit, err := s.EffectiveLimit(email)
	if err != nil {
		return DeviceAllowed, err
	}

	db := database.GetDB()
	now := time.Now().UnixMilli()

	var existing model.ClientDevice
	err = db.Where("client_email = ? AND hwid = ?", email, info.Hwid).First(&existing).Error
	switch {
	case err == nil:
		// Known device: refresh what it reports about itself, and keep it even
		// if the limit has since been lowered — shrinking the cap should stop
		// *new* devices, not break the ones already in use.
		//
		// Descriptive fields are only overwritten when the request actually
		// carried them: apps send the full set on first registration but often
		// just x-hwid afterwards, and blanking the model/OS on every refetch
		// would leave admins with an unidentifiable list of hashes.
		updates := map[string]any{"last_seen": now}
		for column, value := range map[string]string{
			"device_os":    info.DeviceOS,
			"os_version":   info.OSVersion,
			"device_model": info.DeviceModel,
			"user_agent":   info.UserAgent,
			"ip":           info.Ip,
		} {
			if value != "" {
				updates[column] = value
			}
		}
		if e := db.Model(&model.ClientDevice{}).Where("id = ?", existing.Id).Updates(updates).Error; e != nil {
			return DeviceAllowed, e
		}
		return DeviceAllowed, nil
	case !database.IsNotFound(err):
		return DeviceAllowed, err
	}

	if limit > 0 {
		var count int64
		if e := db.Model(&model.ClientDevice{}).Where("client_email = ?", email).Count(&count).Error; e != nil {
			return DeviceAllowed, e
		}
		if count >= int64(limit) {
			return DeviceLimitReached, nil
		}
	}

	device := model.ClientDevice{
		ClientEmail: email,
		Hwid:        info.Hwid,
		DeviceOS:    info.DeviceOS,
		OSVersion:   info.OSVersion,
		DeviceModel: info.DeviceModel,
		UserAgent:   info.UserAgent,
		Ip:          info.Ip,
		FirstSeen:   now,
		LastSeen:    now,
	}
	// Two devices racing on the same free slot both pass the count check; the
	// unique (client_email, hwid) index turns a duplicate into a no-op instead
	// of a 500, and a genuinely new second device is over the cap either way on
	// its next fetch.
	if e := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&device).Error; e != nil {
		return DeviceAllowed, e
	}
	return DeviceAllowed, nil
}

// List returns a client's registered devices, most recently seen first.
func (s *ClientDeviceService) List(email string) ([]model.ClientDevice, error) {
	db := database.GetDB()
	var rows []model.ClientDevice
	err := db.Where("client_email = ?", email).
		Order("last_seen DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Count returns how many devices a client currently has registered.
func (s *ClientDeviceService) Count(email string) (int64, error) {
	db := database.GetDB()
	var count int64
	err := db.Model(&model.ClientDevice{}).Where("client_email = ?", email).Count(&count).Error
	return count, err
}

// CountByEmails returns device counts for many clients in one pass, keyed by
// email. Used by the clients list so every row can show "devices used / cap"
// without a query per row.
func (s *ClientDeviceService) CountByEmails(emails []string) (map[string]int, error) {
	out := make(map[string]int, len(emails))
	if len(emails) == 0 {
		return out, nil
	}
	db := database.GetDB()
	type row struct {
		ClientEmail string
		Total       int
	}
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var rows []row
		err := db.Model(&model.ClientDevice{}).
			Select("client_email, COUNT(*) AS total").
			Where("client_email IN ?", batch).
			Group("client_email").
			Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.ClientEmail] = r.Total
		}
	}
	return out, nil
}

// Delete removes a single device, freeing one slot for the client.
func (s *ClientDeviceService) Delete(email string, id int) error {
	db := database.GetDB()
	return db.Where("client_email = ? AND id = ?", email, id).Delete(&model.ClientDevice{}).Error
}

// Clear removes every device of a client — the "reset devices" action, so a
// user who changed phones can re-register without waiting for an admin to
// hunt down the old row.
func (s *ClientDeviceService) Clear(email string) (int64, error) {
	db := database.GetDB()
	res := db.Where("client_email = ?", email).Delete(&model.ClientDevice{})
	return res.RowsAffected, res.Error
}

// DeleteForEmails drops the device rows of deleted clients. Called from the
// client delete paths so a re-created email never inherits stale devices.
func (s *ClientDeviceService) DeleteForEmails(tx *gorm.DB, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	db := tx
	if db == nil {
		db = database.GetDB()
	}
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		if err := db.Where("client_email IN ?", batch).Delete(&model.ClientDevice{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// RenameEmail moves a client's devices when its email changes, so a rename
// doesn't silently reset the device list.
func (s *ClientDeviceService) RenameEmail(tx *gorm.DB, oldEmail, newEmail string) error {
	if oldEmail == "" || newEmail == "" || oldEmail == newEmail {
		return nil
	}
	db := tx
	if db == nil {
		db = database.GetDB()
	}
	return db.Model(&model.ClientDevice{}).
		Where("client_email = ?", oldEmail).
		Update("client_email", newEmail).Error
}
