package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedPackage(t *testing.T, s *SalesService, name string, price, trafficGB int64, clients int, inbounds string) *model.ResellerPackage {
	t.Helper()
	pkg := &model.ResellerPackage{
		Name:            name,
		Price:           price,
		TrafficGB:       trafficGB,
		ClientQuota:     clients,
		AllowedInbounds: inbounds,
		Enable:          true,
	}
	if err := s.SavePackage(pkg); err != nil {
		t.Fatalf("save package %q: %v", name, err)
	}
	return pkg
}

// TestApprovingAnOrderCreatesTheResellerAccount is the whole point of the sales
// bot: a paid order has to come out the other side as a working, correctly
// scoped reseller login.
func TestApprovingAnOrderCreatesTheResellerAccount(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	inboundSvc := &InboundService{}

	ib := mkInbound(t, 33001, model.VLESS, `{"clients":[]}`)
	pkg := seedPackage(t, sales, "Bronze", 500000, 100, 25, intsToCSV(ib.Id))

	order, err := sales.CreateOrder(555001, "Ali", pkg.Id, model.OrderKindNew)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Price != pkg.Price || order.PackageName != pkg.Name {
		t.Errorf("order should snapshot price and name, got %d/%q", order.Price, order.PackageName)
	}

	// The bot refuses to approve a payment nobody has seen.
	if _, err := sales.ApproveOrder(nil, order.Id, true, inboundSvc, nil); err != ErrNoReceipt {
		t.Errorf("approve without receipt = %v, want ErrNoReceipt", err)
	}
	if _, err := sales.AttachReceipt(order.Id, "file-123"); err != nil {
		t.Fatalf("attach receipt: %v", err)
	}

	result, err := sales.ApproveOrder(nil, order.Id, true, inboundSvc, nil)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !result.IsNew || result.Password == "" {
		t.Fatalf("a first purchase must create an account with a password, got %+v", result)
	}

	account, err := sales.AccountOf(555001)
	if err != nil {
		t.Fatalf("account not linked to the buyer: %v", err)
	}
	if account.Role != model.RoleReseller {
		t.Errorf("role = %q, want reseller", account.Role)
	}
	if account.TrafficQuotaGB != 100 || account.ClientQuota != 25 {
		t.Errorf("quotas = %d GB / %d clients, want the package's", account.TrafficQuotaGB, account.ClientQuota)
	}
	if account.AllowedInbounds != intsToCSV(ib.Id) {
		t.Errorf("allowedInbounds = %q, want the package's inbound", account.AllowedInbounds)
	}

	// Deciding twice must not create a second account.
	if _, err := sales.ApproveOrder(nil, order.Id, true, inboundSvc, nil); err != ErrOrderNotPending {
		t.Errorf("second approve = %v, want ErrOrderNotPending", err)
	}
}

// TestTopUpAddsQuotaToTheSameAccount covers the repeat customer: buying again
// must extend what they have, not hand them a second login.
func TestTopUpAddsQuotaToTheSameAccount(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	inboundSvc := &InboundService{}

	ib1 := mkInbound(t, 33011, model.VLESS, `{"clients":[]}`)
	ib2 := mkInbound(t, 33012, model.VLESS, `{"clients":[]}`)
	first := seedPackage(t, sales, "Bronze", 500000, 100, 25, intsToCSV(ib1.Id))
	second := seedPackage(t, sales, "Silver", 900000, 200, 40, intsToCSV(ib2.Id))

	order, _ := sales.CreateOrder(555002, "Sara", first.Id, model.OrderKindNew)
	_, _ = sales.AttachReceipt(order.Id, "f1")
	if _, err := sales.ApproveOrder(nil, order.Id, true, inboundSvc, nil); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	created, _ := sales.AccountOf(555002)

	renew, err := sales.CreateOrder(555002, "Sara", second.Id, model.OrderKindRenew)
	if err != nil {
		t.Fatalf("create renew order: %v", err)
	}
	_, _ = sales.AttachReceipt(renew.Id, "f2")
	result, err := sales.ApproveOrder(nil, renew.Id, true, inboundSvc, nil)
	if err != nil {
		t.Fatalf("approve renew: %v", err)
	}
	if result.IsNew || result.Password != "" {
		t.Error("a top-up must not create a second account or issue a new password")
	}

	after, _ := sales.AccountOf(555002)
	if after.Id != created.Id {
		t.Fatalf("top-up landed on a different account (%d vs %d)", after.Id, created.Id)
	}
	if after.TrafficQuotaGB != 300 {
		t.Errorf("traffic quota = %d, want 100+200", after.TrafficQuotaGB)
	}
	if after.ClientQuota != 65 {
		t.Errorf("client quota = %d, want 25+40", after.ClientQuota)
	}
	// The second package's inbound is added to what they already had.
	if after.AllowedInbounds != intsToCSV(ib1.Id, ib2.Id) {
		t.Errorf("allowedInbounds = %q, want both inbounds", after.AllowedInbounds)
	}
}

// TestUnlimitedPackageStaysUnlimited: adding a finite budget to an unlimited
// account (or an unlimited package to a finite one) must never silently cap it.
func TestUnlimitedPackageStaysUnlimited(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	inboundSvc := &InboundService{}

	unlimited := seedPackage(t, sales, "Unlimited", 0, 0, 0, "")
	finite := seedPackage(t, sales, "Small", 100000, 50, 10, "")

	order, _ := sales.CreateOrder(555003, "Reza", unlimited.Id, model.OrderKindNew)
	_, _ = sales.AttachReceipt(order.Id, "f1")
	if _, err := sales.ApproveOrder(nil, order.Id, true, inboundSvc, nil); err != nil {
		t.Fatalf("approve: %v", err)
	}

	top, _ := sales.CreateOrder(555003, "Reza", finite.Id, model.OrderKindRenew)
	_, _ = sales.AttachReceipt(top.Id, "f2")
	if _, err := sales.ApproveOrder(nil, top.Id, true, inboundSvc, nil); err != nil {
		t.Fatalf("approve top-up: %v", err)
	}

	after, _ := sales.AccountOf(555003)
	if after.TrafficQuotaGB != 0 || after.ClientQuota != 0 {
		t.Errorf("quotas = %d/%d, want both still unlimited (0)", after.TrafficQuotaGB, after.ClientQuota)
	}
}

// TestApprovingWakesADisabledAccount: paying to top up an account that ran out
// of quota has to bring it back, or the buyer pays and stays cut off.
func TestApprovingWakesADisabledAccount(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	adminSvc := &AdminService{}
	inboundSvc := &InboundService{}

	ib := mkInbound(t, 33021, model.VLESS, `{"clients":[]}`)
	pkg := seedPackage(t, sales, "Bronze", 500000, 100, 25, intsToCSV(ib.Id))

	order, _ := sales.CreateOrder(555004, "Mina", pkg.Id, model.OrderKindNew)
	_, _ = sales.AttachReceipt(order.Id, "f1")
	if _, err := sales.ApproveOrder(nil, order.Id, true, inboundSvc, nil); err != nil {
		t.Fatalf("approve: %v", err)
	}
	account, _ := sales.AccountOf(555004)

	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}
	if err := adminSvc.SetAdminEnabled(actor, account.Id, false, inboundSvc, nil); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if inboundEnabled(t, ib.Id) {
		t.Fatal("the inbound should be down while the account is disabled")
	}

	top, _ := sales.CreateOrder(555004, "Mina", pkg.Id, model.OrderKindRenew)
	_, _ = sales.AttachReceipt(top.Id, "f2")
	if _, err := sales.ApproveOrder(actor, top.Id, true, inboundSvc, nil); err != nil {
		t.Fatalf("approve top-up: %v", err)
	}

	after, _ := sales.AccountOf(555004)
	if after.Disabled {
		t.Error("paying for more quota must re-enable the account")
	}
	if !inboundEnabled(t, ib.Id) {
		t.Error("the buyer's inbound must come back on")
	}
}

// TestRejectedOrderGrantsNothing keeps a turned-down payment from leaking access.
func TestRejectedOrderGrantsNothing(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	inboundSvc := &InboundService{}

	pkg := seedPackage(t, sales, "Bronze", 500000, 100, 25, "")
	order, _ := sales.CreateOrder(555005, "Nima", pkg.Id, model.OrderKindNew)
	_, _ = sales.AttachReceipt(order.Id, "fake-receipt")

	if _, err := sales.RejectOrder(order.Id, "رسید نامعتبر"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := sales.AccountOf(555005); err == nil {
		t.Error("a rejected order must not create an account")
	}
	if _, err := sales.ApproveOrder(nil, order.Id, true, inboundSvc, nil); err != ErrOrderNotPending {
		t.Errorf("approving a rejected order = %v, want ErrOrderNotPending", err)
	}
	if sales.Stats().Revenue != 0 {
		t.Error("a rejected order must not count as revenue")
	}
}

// TestRenewWithoutAnAccountIsRefused: a buyer cannot top up something they
// never bought.
func TestRenewWithoutAnAccountIsRefused(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	pkg := seedPackage(t, sales, "Bronze", 500000, 100, 25, "")

	if _, err := sales.CreateOrder(555006, "Ghost", pkg.Id, model.OrderKindRenew); err != ErrNoAccountToRenew {
		t.Errorf("renew without an account = %v, want ErrNoAccountToRenew", err)
	}
}

// TestStatsCountApprovedOnly keeps the sales report honest.
func TestStatsCountApprovedOnly(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	inboundSvc := &InboundService{}
	pkg := seedPackage(t, sales, "Bronze", 500000, 100, 25, "")

	paid, _ := sales.CreateOrder(555007, "A", pkg.Id, model.OrderKindNew)
	_, _ = sales.AttachReceipt(paid.Id, "f1")
	if _, err := sales.ApproveOrder(nil, paid.Id, true, inboundSvc, nil); err != nil {
		t.Fatalf("approve: %v", err)
	}
	waiting, _ := sales.CreateOrder(555008, "B", pkg.Id, model.OrderKindNew)
	_, _ = sales.AttachReceipt(waiting.Id, "f2")
	// A third buyer who never paid at all.
	_, _ = sales.CreateOrder(555009, "C", pkg.Id, model.OrderKindNew)

	stats := sales.Stats()
	if stats.Revenue != 500000 {
		t.Errorf("revenue = %d, want only the approved order", stats.Revenue)
	}
	if stats.ApprovedOrders != 1 || stats.PendingReview != 1 {
		t.Errorf("counts = %d approved / %d pending, want 1/1", stats.ApprovedOrders, stats.PendingReview)
	}
	if stats.Buyers != 1 {
		t.Errorf("buyers = %d, want only the one who actually bought", stats.Buyers)
	}
}

// TestDeletingAPackageKeepsOrderHistory: prices change, history should not.
func TestDeletingAPackageKeepsOrderHistory(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	pkg := seedPackage(t, sales, "Bronze", 500000, 100, 25, "")

	order, _ := sales.CreateOrder(555010, "D", pkg.Id, model.OrderKindNew)
	if err := sales.DeletePackage(pkg.Id); err != nil {
		t.Fatalf("delete package: %v", err)
	}
	reloaded, err := sales.GetOrder(order.Id)
	if err != nil {
		t.Fatalf("order should survive its package: %v", err)
	}
	if reloaded.PackageName != "Bronze" || reloaded.Price != 500000 {
		t.Errorf("order lost its snapshot: %q / %d", reloaded.PackageName, reloaded.Price)
	}
}

// TestUsernamesDoNotCollide guards the generated login name.
func TestUsernamesDoNotCollide(t *testing.T) {
	setupBulkDB(t)
	sales := &SalesService{}
	adminSvc := &AdminService{}

	// Somebody already holds the name the generator would pick first.
	actor := &model.User{Id: 1, Username: "admin", Role: model.RoleSuperAdmin}
	if _, err := adminSvc.CreateAdmin(actor, "rs555011", "pw", model.RoleReadonly, "", 0, 0); err != nil {
		t.Fatalf("seed clashing user: %v", err)
	}

	pkg := seedPackage(t, sales, "Bronze", 500000, 100, 25, "")
	order, _ := sales.CreateOrder(555011, "E", pkg.Id, model.OrderKindNew)
	_, _ = sales.AttachReceipt(order.Id, "f1")
	result, err := sales.ApproveOrder(nil, order.Id, true, &InboundService{}, nil)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if result.Username == "rs555011" {
		t.Error("the generator handed out a username that was already taken")
	}
	var count int64
	database.GetDB().Model(&model.User{}).Where("username = ?", result.Username).Count(&count)
	if count != 1 {
		t.Errorf("username %q exists %d times", result.Username, count)
	}
}
