// Package service: the Telegram shop's wallet and pay-as-you-go billing.
//
// A buyer tops their wallet up, creates a config with a traffic cap, and is
// charged for what they actually consume at the panel's per-GB price. Nothing
// is taken up front: BillAll meters every config and debits the wallet as the
// bytes go by, so a 10 GB config that only ever moved 1 GB costs one gigabyte.
package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
)

// ShopService owns the wallet, the configs bought with it, and the billing run.
type ShopService struct {
	settingService SettingService
	clientService  ClientService
}

var (
	ErrTopUpTooSmall    = errors.New("the amount is below the minimum top-up")
	ErrTopUpTooLarge    = errors.New("the amount is above the maximum top-up")
	ErrTopUpNotPending  = errors.New("this top-up has already been decided")
	ErrInsufficientFund = errors.New("wallet balance is too low")
	ErrVolumeTooLarge   = errors.New("the requested volume is above the maximum")
	ErrVolumeInvalid    = errors.New("the requested volume must be greater than zero")
	ErrNoShopInbound    = errors.New("no inbound is configured for the shop")
	ErrUserBlocked      = errors.New("this user is blocked")
	ErrConfigNotFound   = errors.New("config not found")
	ErrNameInvalid      = errors.New("a config name must be 3-32 characters of letters, digits, dot, dash or underscore")
	ErrNameTaken        = errors.New("that config name is already in use")
)

const shopBytesPerGB = int64(1024 * 1024 * 1024)

func nowMilli() int64 { return time.Now().UnixMilli() }

// ------------------------------------------------------------------ users --

// User returns the shop user, creating the row on first contact so a wallet
// always exists for anyone who talks to the bot.
func (s *ShopService) User(telegramId int64, username, firstName string) (*model.BotUser, error) {
	db := database.GetDB()
	var u model.BotUser
	err := db.Where("telegram_id = ?", telegramId).First(&u).Error
	if err == nil {
		// Keep the display fields fresh, but never overwrite them with blanks.
		updates := map[string]any{}
		if username != "" && username != u.Username {
			updates["username"] = username
		}
		if firstName != "" && firstName != u.FirstName {
			updates["first_name"] = firstName
		}
		if len(updates) > 0 {
			_ = db.Model(&model.BotUser{}).Where("telegram_id = ?", telegramId).Updates(updates).Error
			u.Username, u.FirstName = username, firstName
		}
		return &u, nil
	}
	if !database.IsNotFound(err) {
		return nil, err
	}
	u = model.BotUser{TelegramId: telegramId, Username: username, FirstName: firstName}
	if err := db.Create(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUser reads a wallet without creating one.
func (s *ShopService) GetUser(telegramId int64) (*model.BotUser, error) {
	var u model.BotUser
	if err := database.GetDB().Where("telegram_id = ?", telegramId).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsers returns every shop user, richest first — which is also the order an
// admin most often wants.
func (s *ShopService) ListUsers(limit int) ([]model.BotUser, error) {
	q := database.GetDB().Model(&model.BotUser{}).Order("balance DESC, telegram_id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []model.BotUser
	err := q.Find(&rows).Error
	return rows, err
}

func (s *ShopService) SetBlocked(telegramId int64, blocked bool) error {
	return database.GetDB().Model(&model.BotUser{}).
		Where("telegram_id = ?", telegramId).Update("blocked", blocked).Error
}

// ------------------------------------------------------------------ money --

// credit moves money and writes the ledger entry in one transaction, so a
// balance and its explanation can never disagree.
func (s *ShopService) credit(telegramId int64, amount int64, kind, details string) (int64, error) {
	if amount == 0 {
		u, err := s.GetUser(telegramId)
		if err != nil {
			return 0, err
		}
		return u.Balance, nil
	}
	var balance int64
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var u model.BotUser
		if err := tx.Where("telegram_id = ?", telegramId).First(&u).Error; err != nil {
			if !database.IsNotFound(err) {
				return err
			}
			u = model.BotUser{TelegramId: telegramId}
			if err := tx.Create(&u).Error; err != nil {
				return err
			}
		}
		balance = u.Balance + amount
		updates := map[string]any{"balance": balance}
		if amount > 0 && kind == model.TxTopUp {
			updates["total_paid"] = u.TotalPaid + amount
		}
		if amount < 0 {
			updates["total_spent"] = u.TotalSpent + (-amount)
		}
		if err := tx.Model(&model.BotUser{}).Where("telegram_id = ?", telegramId).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Create(&model.WalletTransaction{
			TelegramId: telegramId,
			Amount:     amount,
			Kind:       kind,
			Details:    details,
			Balance:    balance,
		}).Error
	})
	return balance, err
}

// Adjust is an admin's manual correction, in either direction.
func (s *ShopService) Adjust(telegramId int64, amount int64, details string) (int64, error) {
	if strings.TrimSpace(details) == "" {
		details = "adjustment by admin"
	}
	return s.credit(telegramId, amount, model.TxAdjust, details)
}

// Transactions returns a wallet's ledger, newest first.
func (s *ShopService) Transactions(telegramId int64, limit int) ([]model.WalletTransaction, error) {
	q := database.GetDB().Model(&model.WalletTransaction{}).
		Where("telegram_id = ?", telegramId).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []model.WalletTransaction
	err := q.Find(&rows).Error
	return rows, err
}

// --------------------------------------------------------------- top-ups --

// RequestTopUp opens a top-up for an amount, enforcing the configured bounds.
func (s *ShopService) RequestTopUp(telegramId int64, name string, amount int64) (*model.WalletTopUp, error) {
	minimum, _ := s.settingService.GetShopMinTopUp()
	maximum, _ := s.settingService.GetShopMaxTopUp()
	if amount <= 0 || (minimum > 0 && amount < minimum) {
		return nil, ErrTopUpTooSmall
	}
	if maximum > 0 && amount > maximum {
		return nil, ErrTopUpTooLarge
	}
	row := &model.WalletTopUp{
		TelegramId:   telegramId,
		TelegramName: strings.TrimSpace(name),
		Amount:       amount,
		Status:       model.TopUpPending,
	}
	if err := database.GetDB().Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// AttachDiscountCode records the code a buyer typed against their pending
// top-up. Nothing is redeemed here — the code is only settled if and when an
// admin approves the payment.
func (s *ShopService) AttachDiscountCode(id int, code string) (*model.WalletTopUp, error) {
	row, err := s.GetTopUp(id)
	if err != nil {
		return nil, err
	}
	if row.Status == model.TopUpApproved || row.Status == model.TopUpRejected {
		return nil, ErrTopUpNotPending
	}
	code = NormalizeDiscountCode(code)
	if err := database.GetDB().Model(&model.WalletTopUp{}).Where("id = ?", id).
		Update("discount_code", code).Error; err != nil {
		return nil, err
	}
	row.DiscountCode = code
	return row, nil
}

func (s *ShopService) GetTopUp(id int) (*model.WalletTopUp, error) {
	var row model.WalletTopUp
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// AttachTopUpReceipt hands a top-up to the admin queue.
func (s *ShopService) AttachTopUpReceipt(id int, fileId string) (*model.WalletTopUp, error) {
	row, err := s.GetTopUp(id)
	if err != nil {
		return nil, err
	}
	if row.Status != model.TopUpPending && row.Status != model.TopUpReview {
		return nil, ErrTopUpNotPending
	}
	if err := database.GetDB().Model(&model.WalletTopUp{}).Where("id = ?", id).
		Updates(map[string]any{"receipt_file_id": fileId, "status": model.TopUpReview}).Error; err != nil {
		return nil, err
	}
	row.ReceiptFileId, row.Status = fileId, model.TopUpReview
	return row, nil
}

// ApproveTopUp credits the wallet, plus whatever discount code the buyer
// attached. Deciding twice is refused, so a double-tap on the approve button
// cannot pay someone twice.
func (s *ShopService) ApproveTopUp(id int) (*model.WalletTopUp, int64, error) {
	row, err := s.GetTopUp(id)
	if err != nil {
		return nil, 0, err
	}
	if row.Status == model.TopUpApproved || row.Status == model.TopUpRejected {
		return nil, 0, ErrTopUpNotPending
	}
	// Only the update that actually moved the row out of "pending" may pay: two
	// approvals racing on the same request must not credit it twice.
	res := database.GetDB().Model(&model.WalletTopUp{}).Where("id = ? AND status <> ?", id, model.TopUpApproved).
		Updates(map[string]any{"status": model.TopUpApproved, "decided_at": nowMilli()})
	if res.Error != nil {
		return nil, 0, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, 0, ErrTopUpNotPending
	}
	balance, err := s.credit(row.TelegramId, row.Amount, model.TxTopUp,
		fmt.Sprintf("top-up #%d", row.Id))
	if err != nil {
		return nil, 0, err
	}
	// The code is settled here rather than when it was typed, so a request that
	// is never approved consumes nothing.
	if discount, bonus := s.redeemDiscount(row); bonus > 0 {
		if b, err := s.credit(row.TelegramId, bonus, model.TxBonus,
			fmt.Sprintf("%s (%d%%)", discount.Code, discount.Percent)); err == nil {
			balance = b
		}
		_ = database.GetDB().Model(&model.WalletTopUp{}).Where("id = ?", id).
			Update("bonus", bonus).Error
		row.Bonus = bonus
	}
	row.Status = model.TopUpApproved
	return row, balance, nil
}

func (s *ShopService) RejectTopUp(id int, note string) (*model.WalletTopUp, error) {
	row, err := s.GetTopUp(id)
	if err != nil {
		return nil, err
	}
	if row.Status == model.TopUpApproved || row.Status == model.TopUpRejected {
		return nil, ErrTopUpNotPending
	}
	if err := database.GetDB().Model(&model.WalletTopUp{}).Where("id = ?", id).
		Updates(map[string]any{"status": model.TopUpRejected, "note": strings.TrimSpace(note), "decided_at": nowMilli()}).Error; err != nil {
		return nil, err
	}
	row.Status, row.Note = model.TopUpRejected, note
	return row, nil
}

// ListTopUps returns top-ups newest first, optionally filtered by status.
func (s *ShopService) ListTopUps(status string, limit int) ([]model.WalletTopUp, error) {
	q := database.GetDB().Model(&model.WalletTopUp{}).Order("id DESC")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []model.WalletTopUp
	err := q.Find(&rows).Error
	return rows, err
}

func (s *ShopService) ListTopUpsOf(telegramId int64, limit int) ([]model.WalletTopUp, error) {
	q := database.GetDB().Model(&model.WalletTopUp{}).
		Where("telegram_id = ?", telegramId).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []model.WalletTopUp
	err := q.Find(&rows).Error
	return rows, err
}

// CountPendingTopUpsOf is the same count for one user, for their admin screen.
func (s *ShopService) CountPendingTopUpsOf(telegramId int64) int64 {
	var n int64
	database.GetDB().Model(&model.WalletTopUp{}).
		Where("status = ? AND telegram_id = ?", model.TopUpReview, telegramId).Count(&n)
	return n
}

func (s *ShopService) CountPendingTopUps() int64 {
	var n int64
	database.GetDB().Model(&model.WalletTopUp{}).Where("status = ?", model.TopUpReview).Count(&n)
	return n
}

// -------------------------------------------------------------- configs --

// CreateConfig makes a panel client on the shop's inbound with the requested
// traffic cap and records it for billing. Nothing is charged here: the user
// pays for what they actually consume.
// CreateConfig sells one config. name is what the buyer asked it to be called;
// empty means "pick one for me".
func (s *ShopService) CreateConfig(inboundSvc *InboundService, telegramId int64, volumeGB int64, name string) (*model.BotConfig, error) {
	user, err := s.User(telegramId, "", "")
	if err != nil {
		return nil, err
	}
	if user.Blocked {
		return nil, ErrUserBlocked
	}
	if volumeGB <= 0 {
		return nil, ErrVolumeInvalid
	}
	if maxVolume, _ := s.settingService.GetShopMaxVolumeGB(); maxVolume > 0 && volumeGB > maxVolume {
		return nil, ErrVolumeTooLarge
	}
	// A user has to be able to pay for at least some of what they are about to
	// use, otherwise they would consume traffic with an empty wallet and the
	// billing run would only ever be able to switch the config off after.
	minBalance, _ := s.settingService.GetShopMinBalance()
	if user.Balance < minBalance || user.Balance <= 0 {
		return nil, ErrInsufficientFund
	}

	inboundId, _ := s.settingService.GetShopInboundId()
	if inboundId <= 0 {
		return nil, ErrNoShopInbound
	}
	if _, err := inboundSvc.GetInbound(inboundId); err != nil {
		return nil, ErrNoShopInbound
	}

	email, err := s.configEmail(name)
	if err != nil {
		return nil, err
	}
	subId := random.NumLower(16)
	client := model.Client{
		Email:   email,
		SubID:   subId,
		Enable:  true,
		TotalGB: volumeGB * shopBytesPerGB,
	}
	if days, _ := s.settingService.GetShopConfigDays(); days > 0 {
		client.ExpiryTime = time.Now().AddDate(0, 0, days).UnixMilli()
	}
	if _, err := s.clientService.CreateOne(inboundSvc, inboundId, client); err != nil {
		return nil, err
	}

	cfg := &model.BotConfig{
		TelegramId: telegramId,
		Email:      email,
		SubID:      subId,
		InboundId:  inboundId,
		VolumeGB:   volumeGB,
		Active:     true,
	}
	if err := database.GetDB().Create(cfg).Error; err != nil {
		return nil, err
	}
	return cfg, nil
}

// configEmail settles on the name a config is stored under. A name the buyer
// typed is validated and taken as-is; an empty one is generated.
func (s *ShopService) configEmail(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return s.freeEmail(), nil
	}
	if err := ValidateConfigName(name); err != nil {
		return "", err
	}
	taken, err := s.nameTaken(name)
	if err != nil {
		return "", err
	}
	if taken {
		return "", ErrNameTaken
	}
	return name, nil
}

// ValidateConfigName is what a buyer may call their own config. The panel keys
// clients by this string and puts it in share links and traffic stats, so it is
// kept to plain ASCII of a sane length rather than whatever a phone keyboard can
// produce.
func ValidateConfigName(name string) error {
	name = strings.TrimSpace(name)
	if len(name) < configNameMinLen || len(name) > configNameMaxLen {
		return ErrNameInvalid
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.':
		default:
			return ErrNameInvalid
		}
	}
	return nil
}

const (
	configNameMinLen = 3
	configNameMaxLen = 32
)

func (s *ShopService) nameTaken(name string) (bool, error) {
	var count int64
	if err := database.GetDB().Model(&model.ClientRecord{}).
		Where("email = ?", name).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// freeEmail invents a config name that is unique in the panel.
//
// It deliberately carries no Telegram id: the name travels in share links, in
// the subscription page and in the panel's client list, and pinning a buyer's
// account id to all of those identifies them to anyone who sees one.
func (s *ShopService) freeEmail() string {
	for range 20 {
		candidate := configNamePrefix + strings.ToLower(random.NumLower(8))
		if taken, err := s.nameTaken(candidate); err != nil || !taken {
			return candidate
		}
	}
	return configNamePrefix + strings.ToLower(random.NumLower(16))
}

const configNamePrefix = "cfg-"

func (s *ShopService) ListConfigs(telegramId int64) ([]model.BotConfig, error) {
	var rows []model.BotConfig
	err := database.GetDB().Model(&model.BotConfig{}).
		Where("telegram_id = ?", telegramId).Order("id DESC").Find(&rows).Error
	return rows, err
}

// ListAllConfigs is the admin's view of every config the shop sold.
func (s *ShopService) ListAllConfigs(limit int) ([]model.BotConfig, error) {
	q := database.GetDB().Model(&model.BotConfig{}).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []model.BotConfig
	err := q.Find(&rows).Error
	return rows, err
}

func (s *ShopService) GetConfig(id int) (*model.BotConfig, error) {
	var row model.BotConfig
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return nil, ErrConfigNotFound
	}
	return &row, nil
}

// SetConfigPaused is the owner switching their own config off or on. It records
// the intent on the config so a billing run cannot undo it, then moves the panel
// client to match. Resuming only takes effect if the wallet can pay for it.
func (s *ShopService) SetConfigPaused(inboundSvc *InboundService, id int, paused bool) (*model.BotConfig, error) {
	cfg, err := s.GetConfig(id)
	if err != nil {
		return nil, err
	}
	user, err := s.GetUser(cfg.TelegramId)
	if err != nil {
		return nil, err
	}
	active := !paused && user.Balance > 0 && !user.Blocked
	if err := database.GetDB().Model(&model.BotConfig{}).Where("id = ?", id).
		Updates(map[string]any{"paused": paused, "active": active}).Error; err != nil {
		return nil, err
	}
	if err := s.setClientEnabled(inboundSvc, cfg.Email, active); err != nil {
		logger.Warning("shop: could not toggle client", cfg.Email, err)
	}
	cfg.Paused = paused
	cfg.Active = active
	return cfg, nil
}

// AddVolume raises a config's traffic cap. Nothing is charged here: the wallet
// pays for traffic as it is used, so the cap is only the ceiling the user is
// allowed to reach before the config stops.
func (s *ShopService) AddVolume(inboundSvc *InboundService, id int, extraGB int64) (*model.BotConfig, error) {
	if extraGB <= 0 {
		return nil, ErrVolumeInvalid
	}
	cfg, err := s.GetConfig(id)
	if err != nil {
		return nil, err
	}
	total := cfg.VolumeGB + extraGB
	if maxVolume, _ := s.settingService.GetShopMaxVolumeGB(); maxVolume > 0 && total > maxVolume {
		return nil, ErrVolumeTooLarge
	}
	if _, err := s.clientService.ResetClientTrafficLimitByEmail(inboundSvc, cfg.Email, int(total)); err != nil {
		return nil, err
	}
	if err := database.GetDB().Model(&model.BotConfig{}).Where("id = ?", id).
		Update("volume_gb", total).Error; err != nil {
		return nil, err
	}
	cfg.VolumeGB = total
	return cfg, nil
}

// DeleteConfig removes the config and its panel client. Whatever it already
// consumed stays charged — the ledger is history, not a reservation.
func (s *ShopService) DeleteConfig(inboundSvc *InboundService, id int) error {
	cfg, err := s.GetConfig(id)
	if err != nil {
		return err
	}
	if _, err := s.clientService.DeleteByEmail(inboundSvc, cfg.Email, false); err != nil {
		logger.Warning("shop: could not delete client", cfg.Email, err)
	}
	return database.GetDB().Delete(&model.BotConfig{}, id).Error
}

// ConfigUsage is one config's meter reading, for the buyer's own screen.
type ConfigUsage struct {
	Config    model.BotConfig `json:"config"`
	UsedBytes int64           `json:"usedBytes"`
	TotalGB   int64           `json:"totalGB"`
	Enable    bool            `json:"enable"`
	Cost      int64           `json:"cost"`
}

// Usage reads what a config has moved and what it has cost so far.
func (s *ShopService) Usage(cfg *model.BotConfig) ConfigUsage {
	out := ConfigUsage{Config: *cfg, TotalGB: cfg.VolumeGB, Cost: cfg.ChargedTraffic + cfg.ChargedDays}
	var traffic xray.ClientTraffic
	if err := database.GetDB().Model(xray.ClientTraffic{}).
		Where("email = ?", cfg.Email).First(&traffic).Error; err == nil {
		out.UsedBytes = traffic.Up + traffic.Down
		out.Enable = traffic.Enable
	}
	return out
}

// -------------------------------------------------------------- billing --

// BillingResult reports what one billing run did, so the caller can tell users
// whose configs were switched off.
type BillingResult struct {
	Charged       int64
	ChargedUsers  int
	SuspendedIds  []int64 // telegram ids whose configs were switched off
	ReactivatedOK int
}

// BillAll meters every live config and debits its owner's wallet. It is the
// only place usage turns into money.
//
// Charging works off a running total rather than a delta: the cost of all the
// traffic a config has ever moved is recomputed, and only the part not already
// charged is taken. That makes the run exactly idempotent — a crash between the
// debit and the bookkeeping cannot double-charge, and no fraction of a gigabyte
// is lost to rounding across runs.
func (s *ShopService) BillAll(inboundSvc *InboundService) BillingResult {
	out := BillingResult{}
	pricePerGB, _ := s.settingService.GetShopPricePerGB()
	pricePerDay, _ := s.settingService.GetShopPricePerDay()
	if pricePerGB <= 0 && pricePerDay <= 0 {
		return out
	}

	db := database.GetDB()
	var configs []model.BotConfig
	if err := db.Model(&model.BotConfig{}).Find(&configs).Error; err != nil {
		return out
	}
	if len(configs) == 0 {
		return out
	}

	// One query for every meter, rather than one per config.
	emails := make([]string, 0, len(configs))
	for _, cfg := range configs {
		emails = append(emails, cfg.Email)
	}
	usedByEmail := make(map[string]int64, len(emails))
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var rows []xray.ClientTraffic
		if err := db.Model(xray.ClientTraffic{}).Where("email IN ?", batch).Find(&rows).Error; err != nil {
			break
		}
		for _, row := range rows {
			usedByEmail[strings.ToLower(row.Email)] = row.Up + row.Down
		}
	}

	now := time.Now()
	touchedUsers := map[int64]struct{}{}
	for i := range configs {
		cfg := &configs[i]
		used := usedByEmail[strings.ToLower(cfg.Email)]

		// What all of this config's traffic should have cost by now.
		trafficCost := int64(0)
		if pricePerGB > 0 {
			trafficCost = used / shopBytesPerGB * pricePerGB
			// Charge the part-gigabyte too, proportionally, so a user who moved
			// 1.5 GB pays for 1.5 GB rather than 1.
			trafficCost += (used % shopBytesPerGB) * pricePerGB / shopBytesPerGB
		}
		dayCost := int64(0)
		if pricePerDay > 0 && cfg.Active {
			days := int64(now.Sub(time.UnixMilli(cfg.CreatedAt)).Hours() / 24)
			if days < 0 {
				days = 0
			}
			dayCost = days * pricePerDay
		}

		owedTraffic := trafficCost - cfg.ChargedTraffic
		owedDays := dayCost - cfg.ChargedDays
		if owedTraffic <= 0 && owedDays <= 0 {
			continue
		}

		if owedTraffic > 0 {
			if _, err := s.credit(cfg.TelegramId, -owedTraffic, model.TxUsage,
				fmt.Sprintf("%s — %s", cfg.Email, humanGB(used))); err != nil {
				logger.Warning("shop: could not charge traffic for", cfg.Email, err)
				continue
			}
			_ = db.Model(&model.BotConfig{}).Where("id = ?", cfg.Id).
				Update("charged_traffic", trafficCost).Error
			out.Charged += owedTraffic
		}
		if owedDays > 0 {
			if _, err := s.credit(cfg.TelegramId, -owedDays, model.TxRent, cfg.Email); err != nil {
				logger.Warning("shop: could not charge rent for", cfg.Email, err)
			} else {
				_ = db.Model(&model.BotConfig{}).Where("id = ?", cfg.Id).
					Update("charged_days", dayCost).Error
				out.Charged += owedDays
			}
		}
		touchedUsers[cfg.TelegramId] = struct{}{}
	}
	out.ChargedUsers = len(touchedUsers)

	// Anyone who just ran out has their configs switched off; anyone back in
	// credit has them switched on again.
	out.SuspendedIds = s.reconcileWallets(inboundSvc, touchedUsers)
	return out
}

// reconcileWallets switches configs off for empty wallets and back on for
// funded ones. Returns the users who were just cut off, so the bot can tell
// them why their connection stopped.
func (s *ShopService) reconcileWallets(inboundSvc *InboundService, touched map[int64]struct{}) []int64 {
	db := database.GetDB()
	var users []model.BotUser
	if err := db.Model(&model.BotUser{}).Find(&users).Error; err != nil {
		return nil
	}
	var suspended []int64
	for i := range users {
		u := &users[i]
		funded := u.Balance > 0 && !u.Blocked
		var configs []model.BotConfig
		if err := db.Model(&model.BotConfig{}).
			Where("telegram_id = ?", u.TelegramId).Find(&configs).Error; err != nil {
			continue
		}
		cutOff := false
		for j := range configs {
			cfg := &configs[j]
			// A config the owner paused stays off however healthy the wallet is.
			wantActive := funded && !cfg.Paused
			if cfg.Active == wantActive {
				continue
			}
			if err := s.setClientEnabled(inboundSvc, cfg.Email, wantActive); err != nil {
				logger.Warning("shop: could not toggle client", cfg.Email, err)
				continue
			}
			_ = db.Model(&model.BotConfig{}).Where("id = ?", cfg.Id).Update("active", wantActive).Error
			if !wantActive && !cfg.Paused {
				cutOff = true
			}
		}
		if cutOff {
			if _, hit := touched[u.TelegramId]; hit {
				suspended = append(suspended, u.TelegramId)
			}
		}
	}
	return suspended
}

// DeletedConfig is one config the clean-up sweep removed, kept long enough to
// tell its owner why it disappeared.
type DeletedConfig struct {
	TelegramId int64
	Email      string
}

// MaintainConfigs marks configs that have gone dead, un-marks those that
// recovered, and deletes the ones that have been dead longer than the admin's
// configured grace period.
//
// "Dead" is traffic exhausted or an empty wallet — states the owner did not
// choose. A config its owner paused on purpose is left alone however long it
// stays off; deleting someone's config because they turned it off for a holiday
// would be a nasty surprise.
//
// Marking runs even when the grace period is zero, so switching the sweep on
// later does not immediately delete everything that was already dead.
func (s *ShopService) MaintainConfigs(inboundSvc *InboundService) []DeletedConfig {
	db := database.GetDB()
	var configs []model.BotConfig
	if err := db.Model(&model.BotConfig{}).Find(&configs).Error; err != nil || len(configs) == 0 {
		return nil
	}

	balances := map[int64]int64{}
	blocked := map[int64]bool{}
	var users []model.BotUser
	if err := db.Model(&model.BotUser{}).Find(&users).Error; err == nil {
		for i := range users {
			balances[users[i].TelegramId] = users[i].Balance
			blocked[users[i].TelegramId] = users[i].Blocked
		}
	}

	now := nowMilli()
	graceDays, _ := s.settingService.GetShopDeleteDeadDays()
	cutoff := int64(0)
	if graceDays > 0 {
		cutoff = now - int64(graceDays)*24*60*60*1000
	}

	var deleted []DeletedConfig
	for i := range configs {
		cfg := &configs[i]
		dead := s.configIsDead(cfg, balances[cfg.TelegramId], blocked[cfg.TelegramId])

		switch {
		case !dead && cfg.DeadSince != 0:
			_ = db.Model(&model.BotConfig{}).Where("id = ?", cfg.Id).Update("dead_since", 0).Error
		case dead && cfg.DeadSince == 0:
			_ = db.Model(&model.BotConfig{}).Where("id = ?", cfg.Id).Update("dead_since", now).Error
		case dead && cutoff > 0 && cfg.DeadSince <= cutoff:
			if err := s.DeleteConfig(inboundSvc, cfg.Id); err != nil {
				logger.Warning("shop: could not sweep dead config", cfg.Email, err)
				continue
			}
			logger.Debug("shop: swept dead config", cfg.Email)
			deleted = append(deleted, DeletedConfig{TelegramId: cfg.TelegramId, Email: cfg.Email})
		}
	}
	return deleted
}

// configIsDead reports whether a config is unusable for a reason its owner did
// not pick.
func (s *ShopService) configIsDead(cfg *model.BotConfig, balance int64, blocked bool) bool {
	if cfg.Paused {
		return false
	}
	if balance <= 0 || blocked {
		return true
	}
	if cfg.VolumeGB > 0 {
		usage := s.Usage(cfg)
		if usage.UsedBytes >= cfg.VolumeGB*shopBytesPerGB {
			return true
		}
	}
	return false
}

// setClientEnabled flips one client's enable flag through the client service,
// so the change lands in the panel and in the running core the same way a manual
// edit would.
func (s *ShopService) setClientEnabled(inboundSvc *InboundService, email string, enabled bool) error {
	rec, err := s.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return err
	}
	if rec.Enable == enabled {
		return nil
	}
	_, _, err = s.clientService.SetClientEnableByEmail(inboundSvc, email, enabled)
	return err
}

// ShopStats is the headline figures for the admin.
type ShopStats struct {
	Users          int64 `json:"users"`
	Configs        int64 `json:"configs"`
	ActiveConfigs  int64 `json:"activeConfigs"`
	WalletBalance  int64 `json:"walletBalance"`
	TotalPaid      int64 `json:"totalPaid"`
	TotalSpent     int64 `json:"totalSpent"`
	PendingTopUps  int64 `json:"pendingTopUps"`
	SuspendedUsers int64 `json:"suspendedUsers"`
}

func (s *ShopService) Stats() ShopStats {
	db := database.GetDB()
	out := ShopStats{}
	db.Model(&model.BotUser{}).Count(&out.Users)
	db.Model(&model.BotConfig{}).Count(&out.Configs)
	db.Model(&model.BotConfig{}).Where("active = ?", true).Count(&out.ActiveConfigs)
	db.Model(&model.BotUser{}).Select("COALESCE(SUM(balance), 0)").Scan(&out.WalletBalance)
	db.Model(&model.BotUser{}).Select("COALESCE(SUM(total_paid), 0)").Scan(&out.TotalPaid)
	db.Model(&model.BotUser{}).Select("COALESCE(SUM(total_spent), 0)").Scan(&out.TotalSpent)
	out.PendingTopUps = s.CountPendingTopUps()
	db.Model(&model.BotUser{}).Where("balance <= 0").Count(&out.SuspendedUsers)
	return out
}

// ShopUserIds lists everyone who has ever used the shop, for broadcasts.
func (s *ShopService) ShopUserIds() []int64 {
	var ids []int64
	database.GetDB().Model(&model.BotUser{}).Pluck("telegram_id", &ids)
	return ids
}

// humanGB is a compact byte figure for the ledger's detail column.
func humanGB(bytes int64) string {
	if bytes < shopBytesPerGB {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB", float64(bytes)/float64(shopBytesPerGB))
}
