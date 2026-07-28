package service

import (
	"errors"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
)

var (
	ErrDiscountNotFound = errors.New("no such discount code")
	ErrDiscountExpired  = errors.New("this discount code has expired")
	ErrDiscountUsedUp   = errors.New("this discount code has been fully redeemed")
	ErrDiscountAlready  = errors.New("this discount code has already been used by this account")
	ErrDiscountInvalid  = errors.New("a discount code needs a name and a percentage between 1 and 100")
	ErrDiscountExists   = errors.New("a discount code with that name already exists")
)

// NormalizeDiscountCode is how a code is stored and compared. Buyers type codes
// in whatever case and with stray spaces; the shop owner should not have to care.
func NormalizeDiscountCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// CreateDiscount registers a new code.
func (s *ShopService) CreateDiscount(code string, percent int, maxBonus int64, maxUses int, expiresAt int64) (*model.DiscountCode, error) {
	code = NormalizeDiscountCode(code)
	if code == "" || percent <= 0 || percent > 100 {
		return nil, ErrDiscountInvalid
	}
	var count int64
	if err := database.GetDB().Model(&model.DiscountCode{}).Where("code = ?", code).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrDiscountExists
	}
	row := &model.DiscountCode{
		Code:      code,
		Percent:   percent,
		MaxBonus:  max(maxBonus, 0),
		MaxUses:   max(maxUses, 0),
		ExpiresAt: expiresAt,
		Enabled:   true,
	}
	if err := database.GetDB().Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *ShopService) ListDiscounts(limit int) ([]model.DiscountCode, error) {
	q := database.GetDB().Model(&model.DiscountCode{}).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []model.DiscountCode
	err := q.Find(&rows).Error
	return rows, err
}

func (s *ShopService) GetDiscount(id int) (*model.DiscountCode, error) {
	var row model.DiscountCode
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return nil, ErrDiscountNotFound
	}
	return &row, nil
}

func (s *ShopService) SetDiscountEnabled(id int, enabled bool) (*model.DiscountCode, error) {
	row, err := s.GetDiscount(id)
	if err != nil {
		return nil, err
	}
	if err := database.GetDB().Model(&model.DiscountCode{}).Where("id = ?", id).
		Update("enabled", enabled).Error; err != nil {
		return nil, err
	}
	row.Enabled = enabled
	return row, nil
}

// DeleteDiscount removes a code. Redemptions are left alone — they are history,
// and the ledger entries they produced still have to be explainable.
func (s *ShopService) DeleteDiscount(id int) error {
	return database.GetDB().Delete(&model.DiscountCode{}, id).Error
}

// ValidateDiscount is the check a buyer's typed code goes through before it is
// attached to a top-up. It returns the code and the bonus it would be worth on
// this amount, or the reason it cannot be used.
func (s *ShopService) ValidateDiscount(code string, telegramId int64, amount int64) (*model.DiscountCode, int64, error) {
	code = NormalizeDiscountCode(code)
	if code == "" {
		return nil, 0, ErrDiscountNotFound
	}
	var row model.DiscountCode
	if err := database.GetDB().Where("code = ?", code).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, ErrDiscountNotFound
		}
		return nil, 0, err
	}
	if !row.Enabled {
		return nil, 0, ErrDiscountNotFound
	}
	if row.ExpiresAt > 0 && nowMilli() > row.ExpiresAt {
		return nil, 0, ErrDiscountExpired
	}
	if row.MaxUses > 0 && row.Used >= row.MaxUses {
		return nil, 0, ErrDiscountUsedUp
	}
	var mine int64
	if err := database.GetDB().Model(&model.DiscountRedemption{}).
		Where("code = ? AND telegram_id = ?", row.Code, telegramId).Count(&mine).Error; err != nil {
		return nil, 0, err
	}
	if mine > 0 {
		return nil, 0, ErrDiscountAlready
	}
	return &row, DiscountBonus(&row, amount), nil
}

// DiscountBonus is the extra credit a code is worth on an amount, after its cap.
func DiscountBonus(code *model.DiscountCode, amount int64) int64 {
	if code == nil || amount <= 0 || code.Percent <= 0 {
		return 0
	}
	bonus := amount * int64(code.Percent) / 100
	if code.MaxBonus > 0 && bonus > code.MaxBonus {
		bonus = code.MaxBonus
	}
	return bonus
}

// redeemDiscount consumes one use of a code for a top-up that has just been
// approved. It is deliberately conservative: a code that turned out to be
// exhausted, expired or already used between the buyer typing it and the admin
// approving it grants nothing rather than failing the top-up, because the money
// itself has already been paid.
func (s *ShopService) redeemDiscount(row *model.WalletTopUp) (*model.DiscountCode, int64) {
	code := NormalizeDiscountCode(row.DiscountCode)
	if code == "" {
		return nil, 0
	}
	discount, bonus, err := s.ValidateDiscount(code, row.TelegramId, row.Amount)
	if err != nil || bonus <= 0 {
		return nil, 0
	}
	db := database.GetDB()
	// Claim a use under a condition, so two approvals racing on the last use of
	// a code cannot both win it.
	claim := db.Model(&model.DiscountCode{}).Where("id = ?", discount.Id)
	if discount.MaxUses > 0 {
		claim = claim.Where("used < ?", discount.MaxUses)
	}
	res := claim.UpdateColumn("used", gorm.Expr("used + 1"))
	if res.Error != nil || res.RowsAffected == 0 {
		return nil, 0
	}
	if err := db.Create(&model.DiscountRedemption{
		Code:       discount.Code,
		TelegramId: row.TelegramId,
		TopUpId:    row.Id,
		Bonus:      bonus,
	}).Error; err != nil {
		return nil, 0
	}
	return discount, bonus
}
