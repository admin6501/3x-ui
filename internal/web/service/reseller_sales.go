// Package service: reseller sales. Packages are the price list the Telegram
// sales bot shows; orders are what buyers put through it. Approving an order is
// the only place that turns money into access, so it lives here rather than in
// the bot — the panel UI drives exactly the same code path.
package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
)

// SalesService manages reseller packages and the orders placed against them.
type SalesService struct {
	adminService AdminService
}

var (
	ErrPackageNotFound  = errors.New("package not found")
	ErrOrderNotFound    = errors.New("order not found")
	ErrOrderNotPending  = errors.New("this order has already been decided")
	ErrNoReceipt        = errors.New("the order has no payment receipt attached")
	ErrNoAccountToRenew = errors.New("this buyer has no reseller account to top up")
)

// ---------------------------------------------------------------- packages --

// ListPackages returns every package, the way the panel lists them.
func (s *SalesService) ListPackages() ([]model.ResellerPackage, error) {
	var rows []model.ResellerPackage
	err := database.GetDB().Model(&model.ResellerPackage{}).
		Order("sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

// ListPackagesForSale returns only what a buyer may actually order.
func (s *SalesService) ListPackagesForSale() ([]model.ResellerPackage, error) {
	var rows []model.ResellerPackage
	err := database.GetDB().Model(&model.ResellerPackage{}).
		Where("enable = ?", true).Order("sort_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (s *SalesService) GetPackage(id int) (*model.ResellerPackage, error) {
	var row model.ResellerPackage
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return nil, ErrPackageNotFound
	}
	return &row, nil
}

func (s *SalesService) SavePackage(p *model.ResellerPackage) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return errors.New("package name is required")
	}
	if p.Price < 0 {
		p.Price = 0
	}
	if p.TrafficGB < 0 {
		p.TrafficGB = 0
	}
	if p.ClientQuota < 0 {
		p.ClientQuota = 0
	}
	if p.DurationDays < 0 {
		p.DurationDays = 0
	}
	p.AllowedInbounds = NormalizeAllowedInbounds(p.AllowedInbounds)
	db := database.GetDB()
	if p.Id > 0 {
		return db.Model(&model.ResellerPackage{}).Where("id = ?", p.Id).Updates(map[string]any{
			"name":             p.Name,
			"description":      p.Description,
			"price":            p.Price,
			"traffic_gb":       p.TrafficGB,
			"client_quota":     p.ClientQuota,
			"duration_days":    p.DurationDays,
			"allowed_inbounds": p.AllowedInbounds,
			"enable":           p.Enable,
			"sort_order":       p.SortOrder,
		}).Error
	}
	return db.Create(p).Error
}

// DeletePackage removes a package. Orders keep working: they snapshot the name
// and price they were placed at.
func (s *SalesService) DeletePackage(id int) error {
	return database.GetDB().Delete(&model.ResellerPackage{}, id).Error
}

func (s *SalesService) SetPackageEnabled(id int, enabled bool) error {
	return database.GetDB().Model(&model.ResellerPackage{}).
		Where("id = ?", id).Update("enable", enabled).Error
}

// ------------------------------------------------------------------ orders --

// ListOrders returns orders newest first, optionally filtered by status.
func (s *SalesService) ListOrders(status string, limit int) ([]model.ResellerOrder, error) {
	q := database.GetDB().Model(&model.ResellerOrder{}).Order("id DESC")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []model.ResellerOrder
	err := q.Find(&rows).Error
	return rows, err
}

// ListOrdersOf returns one buyer's own order history.
func (s *SalesService) ListOrdersOf(telegramId int64, limit int) ([]model.ResellerOrder, error) {
	q := database.GetDB().Model(&model.ResellerOrder{}).
		Where("telegram_id = ?", telegramId).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []model.ResellerOrder
	err := q.Find(&rows).Error
	return rows, err
}

func (s *SalesService) GetOrder(id int) (*model.ResellerOrder, error) {
	var row model.ResellerOrder
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return nil, ErrOrderNotFound
	}
	return &row, nil
}

// CountPendingReview is what the admin badge counts.
func (s *SalesService) CountPendingReview() int64 {
	var n int64
	database.GetDB().Model(&model.ResellerOrder{}).Where("status = ?", model.OrderReview).Count(&n)
	return n
}

// CreateOrder opens an order for a buyer. Kind is "new" for a first purchase or
// "renew" to top up the account they already have.
func (s *SalesService) CreateOrder(telegramId int64, telegramName string, packageId int, kind string) (*model.ResellerOrder, error) {
	pkg, err := s.GetPackage(packageId)
	if err != nil {
		return nil, err
	}
	if !pkg.Enable {
		return nil, ErrPackageNotFound
	}
	if kind != model.OrderKindRenew {
		kind = model.OrderKindNew
	}
	if kind == model.OrderKindRenew {
		if _, err := s.AccountOf(telegramId); err != nil {
			return nil, ErrNoAccountToRenew
		}
	}
	order := &model.ResellerOrder{
		TelegramId:   telegramId,
		TelegramName: strings.TrimSpace(telegramName),
		PackageId:    pkg.Id,
		PackageName:  pkg.Name,
		Price:        pkg.Price,
		Kind:         kind,
		Status:       model.OrderPending,
	}
	if err := database.GetDB().Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

// AttachReceipt records the buyer's payment proof and hands the order to the
// admin queue.
func (s *SalesService) AttachReceipt(orderId int, fileId string) (*model.ResellerOrder, error) {
	order, err := s.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	if order.Status != model.OrderPending && order.Status != model.OrderReview {
		return nil, ErrOrderNotPending
	}
	if err := database.GetDB().Model(&model.ResellerOrder{}).Where("id = ?", orderId).
		Updates(map[string]any{"receipt_file_id": fileId, "status": model.OrderReview}).Error; err != nil {
		return nil, err
	}
	order.ReceiptFileId = fileId
	order.Status = model.OrderReview
	return order, nil
}

// CancelOrder drops an order the buyer abandoned before paying.
func (s *SalesService) CancelOrder(orderId int, telegramId int64) error {
	res := database.GetDB().Where("id = ? AND telegram_id = ? AND status = ?", orderId, telegramId, model.OrderPending).
		Delete(&model.ResellerOrder{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrOrderNotPending
	}
	return nil
}

// AccountOf returns the reseller account bought by a Telegram user.
func (s *SalesService) AccountOf(telegramId int64) (*model.User, error) {
	var u model.User
	err := database.GetDB().Where("telegram_id = ?", telegramId).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ApprovalResult is what the caller needs to tell the buyer: for a first
// purchase the credentials, for a top-up the new totals.
type ApprovalResult struct {
	Order       *model.ResellerOrder
	Username    string
	Password    string // only set when the account was just created
	IsNew       bool
	TrafficGB   int64
	ClientQuota int
}

// ApproveOrder turns a paid order into access. A first purchase creates a
// reseller account scoped to the package's inbounds and carrying its quotas; a
// top-up adds the package's quotas to the account the buyer already has, which
// also lets the quota enforcer bring their inbounds back on its own.
//
// requireReceipt is on for the bot (an admin should not approve a payment they
// never saw) and can be relaxed by the panel for a manual, out-of-band sale.
func (s *SalesService) ApproveOrder(actor *model.User, orderId int, requireReceipt bool, inboundSvc *InboundService, xrayService *XrayService) (*ApprovalResult, error) {
	order, err := s.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	if order.Status == model.OrderApproved || order.Status == model.OrderRejected {
		return nil, ErrOrderNotPending
	}
	if requireReceipt && order.ReceiptFileId == "" {
		return nil, ErrNoReceipt
	}
	pkg, err := s.GetPackage(order.PackageId)
	if err != nil {
		return nil, err
	}

	db := database.GetDB()
	result := &ApprovalResult{Order: order}

	existing, findErr := s.AccountOf(order.TelegramId)
	if order.Kind == model.OrderKindRenew && findErr != nil {
		return nil, ErrNoAccountToRenew
	}

	if findErr == nil && existing != nil {
		// Top up the account this buyer already has. Quotas add up, so buying
		// the same package twice doubles the budget rather than resetting it.
		updates := map[string]any{}
		newTraffic := existing.TrafficQuotaGB
		newClients := existing.ClientQuota
		// A package with an unlimited quota makes the account unlimited; adding
		// to a quota that is already unlimited would silently cap it.
		if pkg.TrafficGB == 0 || existing.TrafficQuotaGB == 0 {
			newTraffic = 0
		} else {
			newTraffic = existing.TrafficQuotaGB + pkg.TrafficGB
		}
		if pkg.ClientQuota == 0 || existing.ClientQuota == 0 {
			newClients = 0
		} else {
			newClients = existing.ClientQuota + pkg.ClientQuota
		}
		updates["traffic_quota_gb"] = newTraffic
		updates["client_quota"] = newClients
		// A package may widen which inbounds the reseller may use.
		merged := mergeInboundCSV(existing.AllowedInbounds, pkg.AllowedInbounds)
		if merged != existing.AllowedInbounds {
			updates["allowed_inbounds"] = merged
		}
		if err := db.Model(&model.User{}).Where("id = ?", existing.Id).Updates(updates).Error; err != nil {
			return nil, err
		}
		// Paying for more traffic should bring a quota-suspended account back
		// without waiting for the next enforcement tick.
		if existing.Disabled {
			if err := s.adminService.SetAdminEnabled(actor, existing.Id, true, inboundSvc, xrayService); err != nil {
				return nil, err
			}
		}
		s.adminService.EnforceResellerQuotas(inboundSvc)

		result.Username = existing.Username
		result.IsNew = false
		result.TrafficGB = newTraffic
		result.ClientQuota = newClients
		order.PanelUserId = existing.Id
		order.PanelUsername = existing.Username
	} else {
		username := s.freeUsername(order.TelegramId)
		password := random.Seq(12)
		created, err := s.adminService.CreateAdmin(actor, username, password, model.RoleReseller,
			pkg.AllowedInbounds, pkg.TrafficGB, pkg.ClientQuota)
		if err != nil {
			return nil, err
		}
		if err := db.Model(&model.User{}).Where("id = ?", created.Id).
			Update("telegram_id", order.TelegramId).Error; err != nil {
			return nil, err
		}
		result.Username = username
		result.Password = password
		result.IsNew = true
		result.TrafficGB = pkg.TrafficGB
		result.ClientQuota = pkg.ClientQuota
		order.PanelUserId = created.Id
		order.PanelUsername = username
	}

	if err := db.Model(&model.ResellerOrder{}).Where("id = ?", order.Id).Updates(map[string]any{
		"status":         model.OrderApproved,
		"panel_user_id":  order.PanelUserId,
		"panel_username": order.PanelUsername,
		"decided_at":     nowMilli(),
	}).Error; err != nil {
		return nil, err
	}
	order.Status = model.OrderApproved
	return result, nil
}

// RejectOrder turns down a payment, with an optional reason for the buyer.
func (s *SalesService) RejectOrder(orderId int, note string) (*model.ResellerOrder, error) {
	order, err := s.GetOrder(orderId)
	if err != nil {
		return nil, err
	}
	if order.Status == model.OrderApproved || order.Status == model.OrderRejected {
		return nil, ErrOrderNotPending
	}
	if err := database.GetDB().Model(&model.ResellerOrder{}).Where("id = ?", orderId).Updates(map[string]any{
		"status":     model.OrderRejected,
		"note":       strings.TrimSpace(note),
		"decided_at": nowMilli(),
	}).Error; err != nil {
		return nil, err
	}
	order.Status = model.OrderRejected
	order.Note = note
	return order, nil
}

// SalesStats is the headline figures for the admin's sales report.
type SalesStats struct {
	Revenue        int64 `json:"revenue"`
	ApprovedOrders int64 `json:"approvedOrders"`
	PendingReview  int64 `json:"pendingReview"`
	RejectedOrders int64 `json:"rejectedOrders"`
	Buyers         int64 `json:"buyers"`
	Resellers      int64 `json:"resellers"`
}

// Stats sums approved orders only: money that was never approved was never
// earned.
func (s *SalesService) Stats() SalesStats {
	db := database.GetDB()
	out := SalesStats{}
	db.Model(&model.ResellerOrder{}).Where("status = ?", model.OrderApproved).
		Select("COALESCE(SUM(price), 0)").Scan(&out.Revenue)
	db.Model(&model.ResellerOrder{}).Where("status = ?", model.OrderApproved).Count(&out.ApprovedOrders)
	db.Model(&model.ResellerOrder{}).Where("status = ?", model.OrderReview).Count(&out.PendingReview)
	db.Model(&model.ResellerOrder{}).Where("status = ?", model.OrderRejected).Count(&out.RejectedOrders)
	db.Model(&model.ResellerOrder{}).Where("status = ?", model.OrderApproved).
		Distinct("telegram_id").Count(&out.Buyers)
	db.Model(&model.User{}).Where("telegram_id <> 0").Count(&out.Resellers)
	return out
}

// BuyerIds lists everyone who has ever ordered, for the broadcast feature.
func (s *SalesService) BuyerIds() []int64 {
	var ids []int64
	database.GetDB().Model(&model.ResellerOrder{}).
		Distinct("telegram_id").Pluck("telegram_id", &ids)
	return ids
}

// freeUsername builds a panel username from the buyer's Telegram id, adding a
// short suffix if that name is somehow taken.
func (s *SalesService) freeUsername(telegramId int64) string {
	base := "rs" + strconv.FormatInt(telegramId, 10)
	if len(base) > 24 {
		base = base[:24]
	}
	db := database.GetDB()
	candidate := base
	for range 20 {
		var count int64
		if err := db.Model(&model.User{}).Where("username = ?", candidate).Count(&count).Error; err != nil {
			break
		}
		if count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%s", base, strings.ToLower(random.NumLower(3)))
	}
	return fmt.Sprintf("%s_%s", base, strings.ToLower(random.NumLower(6)))
}

func nowMilli() int64 { return time.Now().UnixMilli() }

// mergeInboundCSV unions two allowed-inbound lists, normalised.
func mergeInboundCSV(a, b string) string {
	joined := strings.TrimSpace(a)
	if strings.TrimSpace(b) != "" {
		if joined == "" {
			joined = b
		} else {
			joined = joined + "," + b
		}
	}
	return NormalizeAllowedInbounds(joined)
}

// ListSoldAccounts returns the reseller accounts that came from the bot,
// newest first — the ones the shop owner actually manages.
func (s *SalesService) ListSoldAccounts() ([]model.User, error) {
	var rows []model.User
	err := database.GetDB().Model(&model.User{}).
		Where("telegram_id <> 0").Order("id DESC").Find(&rows).Error
	return rows, err
}

// AccountByID loads one panel account by its numeric id.
func (s *SalesService) AccountByID(id int) (*model.User, error) {
	var u model.User
	if err := database.GetDB().First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}
