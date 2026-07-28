package service

import (
	"errors"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// approvedTopUpWithCode runs the whole buyer path for one code: request, attach,
// approve. It returns the balance the wallet ended on.
func approvedTopUpWithCode(t *testing.T, shop *ShopService, telegramId int64, amount int64, code string) (*model.WalletTopUp, int64) {
	t.Helper()
	row, err := shop.RequestTopUp(telegramId, "buyer", amount)
	if err != nil {
		t.Fatalf("request top-up: %v", err)
	}
	if code != "" {
		if _, err := shop.AttachDiscountCode(row.Id, code); err != nil {
			t.Fatalf("attach code: %v", err)
		}
	}
	approved, balance, err := shop.ApproveTopUp(row.Id)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	return approved, balance
}

// TestDiscountCodeAddsItsPercentOnApproval is the feature in one line: pay
// 100,000 with a 20% code, get 120,000 credited.
func TestDiscountCodeAddsItsPercentOnApproval(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	setShop(t, 2000, 0, 1, nil)

	if _, err := shop.CreateDiscount("nowruz", 20, 0, 0, 0); err != nil {
		t.Fatalf("create discount: %v", err)
	}
	_, _ = shop.User(910001, "", "")

	// The code is stored and matched case-insensitively.
	row, balance := approvedTopUpWithCode(t, shop, 910001, 100000, " NoWruz ")
	if row.Bonus != 20000 {
		t.Errorf("bonus = %d, want 20000", row.Bonus)
	}
	if balance != 120000 {
		t.Errorf("balance = %d, want 120000", balance)
	}

	// Both movements are on the ledger, so the balance stays explainable.
	entries, err := shop.Transactions(910001, 10)
	if err != nil {
		t.Fatalf("transactions: %v", err)
	}
	var sawTopUp, sawBonus bool
	for _, e := range entries {
		switch e.Kind {
		case model.TxTopUp:
			sawTopUp = e.Amount == 100000
		case model.TxBonus:
			sawBonus = e.Amount == 20000
		}
	}
	if !sawTopUp || !sawBonus {
		t.Errorf("ledger is missing an entry: topup=%v bonus=%v (%+v)", sawTopUp, sawBonus, entries)
	}
}

// TestDiscountCodeIsOncePerUser: a code is a promotion, not a standing discount.
func TestDiscountCodeIsOncePerUser(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	setShop(t, 2000, 0, 1, nil)

	if _, err := shop.CreateDiscount("ONCE", 50, 0, 0, 0); err != nil {
		t.Fatalf("create discount: %v", err)
	}
	_, _ = shop.User(910002, "", "")
	_, _ = shop.User(910003, "", "")

	if _, balance := approvedTopUpWithCode(t, shop, 910002, 10000, "ONCE"); balance != 15000 {
		t.Fatalf("first redemption balance = %d, want 15000", balance)
	}
	if _, _, err := shop.ValidateDiscount("ONCE", 910002, 10000); !errors.Is(err, ErrDiscountAlready) {
		t.Errorf("second use by the same buyer returned %v, want ErrDiscountAlready", err)
	}
	// A second approval anyway must not pay the bonus again.
	row, balance := approvedTopUpWithCode(t, shop, 910002, 10000, "ONCE")
	if row.Bonus != 0 {
		t.Errorf("bonus on a reused code = %d, want 0", row.Bonus)
	}
	if balance != 25000 {
		t.Errorf("balance = %d, want 25000 (top-up only)", balance)
	}
	// A different buyer may still use it.
	if _, _, err := shop.ValidateDiscount("ONCE", 910003, 10000); err != nil {
		t.Errorf("another buyer was refused the code: %v", err)
	}
}

// TestDiscountCodeRespectsItsLimits covers the three ways a code stops working.
func TestDiscountCodeRespectsItsLimits(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	setShop(t, 2000, 0, 1, nil)

	// Total-uses cap.
	if _, err := shop.CreateDiscount("TWICE", 10, 0, 2, 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i, id := range []int64{910011, 910012} {
		_, _ = shop.User(id, "", "")
		if row, _ := approvedTopUpWithCode(t, shop, id, 10000, "TWICE"); row.Bonus != 1000 {
			t.Fatalf("redemption %d gave bonus %d, want 1000", i+1, row.Bonus)
		}
	}
	_, _ = shop.User(910013, "", "")
	if _, _, err := shop.ValidateDiscount("TWICE", 910013, 10000); !errors.Is(err, ErrDiscountUsedUp) {
		t.Errorf("third use returned %v, want ErrDiscountUsedUp", err)
	}

	// Expiry.
	past := time.Now().AddDate(0, 0, -1).UnixMilli()
	if _, err := shop.CreateDiscount("OLD", 10, 0, 0, past); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := shop.ValidateDiscount("OLD", 910013, 10000); !errors.Is(err, ErrDiscountExpired) {
		t.Errorf("expired code returned %v, want ErrDiscountExpired", err)
	}

	// Switched off.
	off, err := shop.CreateDiscount("OFF", 10, 0, 0, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := shop.SetDiscountEnabled(off.Id, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, _, err := shop.ValidateDiscount("OFF", 910013, 10000); !errors.Is(err, ErrDiscountNotFound) {
		t.Errorf("disabled code returned %v, want ErrDiscountNotFound", err)
	}
}

// TestDiscountBonusIsCapped: a percentage on an unbounded top-up can be a lot
// more than the owner meant to give away.
func TestDiscountBonusIsCapped(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	setShop(t, 2000, 0, 1, nil)

	code, err := shop.CreateDiscount("CAPPED", 50, 30000, 0, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := DiscountBonus(code, 40000); got != 20000 {
		t.Errorf("under the cap: bonus = %d, want 20000", got)
	}
	if got := DiscountBonus(code, 1000000); got != 30000 {
		t.Errorf("over the cap: bonus = %d, want 30000", got)
	}
}

// TestUnapprovedTopUpConsumesNothing: a code is settled at approval, so a buyer
// cannot burn a limited code's capacity by typing it and walking away.
func TestUnapprovedTopUpConsumesNothing(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	setShop(t, 2000, 0, 1, nil)

	if _, err := shop.CreateDiscount("HOLD", 10, 0, 1, 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _ = shop.User(910021, "", "")
	_, _ = shop.User(910022, "", "")

	row, err := shop.RequestTopUp(910021, "buyer", 10000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := shop.AttachDiscountCode(row.Id, "HOLD"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := shop.RejectTopUp(row.Id, "no receipt"); err != nil {
		t.Fatalf("reject: %v", err)
	}

	// The code's single use is still available to someone else.
	if _, _, err := shop.ValidateDiscount("HOLD", 910022, 10000); err != nil {
		t.Errorf("an abandoned request consumed the code: %v", err)
	}
}

// TestApprovingTwiceCreditsOnce guards the double-tap on the approve button.
func TestApprovingTwiceCreditsOnce(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}
	setShop(t, 2000, 0, 1, nil)
	_, _ = shop.User(910031, "", "")

	row, err := shop.RequestTopUp(910031, "buyer", 50000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, _, err := shop.ApproveTopUp(row.Id); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if _, _, err := shop.ApproveTopUp(row.Id); !errors.Is(err, ErrTopUpNotPending) {
		t.Errorf("second approve returned %v, want ErrTopUpNotPending", err)
	}
	if got := balanceOf(t, shop, 910031); got != 50000 {
		t.Errorf("balance = %d, want 50000 — approving twice paid twice", got)
	}
}

// TestDiscountCodesAreUniqueAndValidated covers the shop owner's side.
func TestDiscountCodesAreUniqueAndValidated(t *testing.T) {
	setupBulkDB(t)
	shop := &ShopService{}

	if _, err := shop.CreateDiscount("DUP", 10, 0, 0, 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := shop.CreateDiscount("dup", 20, 0, 0, 0); !errors.Is(err, ErrDiscountExists) {
		t.Errorf("duplicate (different case) returned %v, want ErrDiscountExists", err)
	}
	for _, percent := range []int{0, -5, 101} {
		if _, err := shop.CreateDiscount("BAD", percent, 0, 0, 0); !errors.Is(err, ErrDiscountInvalid) {
			t.Errorf("percent %d returned %v, want ErrDiscountInvalid", percent, err)
		}
	}
	if _, err := shop.CreateDiscount("   ", 10, 0, 0, 0); !errors.Is(err, ErrDiscountInvalid) {
		t.Errorf("a blank code was accepted")
	}

	// Deleting leaves the redemption history alone.
	code, _ := shop.CreateDiscount("GONE", 10, 0, 0, 0)
	_, _ = shop.User(910041, "", "")
	approvedTopUpWithCode(t, shop, 910041, 10000, "GONE")
	if err := shop.DeleteDiscount(code.Id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var redemptions int64
	database.GetDB().Model(&model.DiscountRedemption{}).Where("code = ?", "GONE").Count(&redemptions)
	if redemptions != 1 {
		t.Errorf("redemption history = %d rows, want 1", redemptions)
	}
}
