package sub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// The sales bot asks for a client's links with no host, because it has no
// incoming request to take one from. These cases pin what the panel then
// resolves — the same chain its own share/QR button uses. They live in this
// package because that is where the real link provider is registered.
func withLinkProvider(t *testing.T) *service.InboundService {
	t.Helper()
	initSubDB(t)
	service.RegisterSubLinkProvider(NewLinkProvider())
	return &service.InboundService{}
}

func seedClient(t *testing.T, ib *model.Inbound, email, uuid string) {
	t.Helper()
	db := database.GetDB()
	settings, _ := json.Marshal(map[string]any{"clients": []map[string]any{
		{"email": email, "id": uuid, "enable": true, "subId": "sub" + email},
	}})
	ib.Settings = string(settings)
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	rec := &model.ClientRecord{Email: email, SubID: "sub" + email, UUID: uuid, Enable: true}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatalf("attach client: %v", err)
	}
}

// TestShopLinksUseTheInboundsOwnAddress: with the inbound bound to a reachable
// address, that address is what the buyer's link names — the bot supplies none.
func TestShopLinksUseTheInboundsOwnAddress(t *testing.T) {
	svc := withLinkProvider(t)
	ib := &model.Inbound{
		UserId: 1, Tag: "shop-listen", Enable: true, Port: 43001, Protocol: model.VLESS,
		Listen: "203.0.113.9", StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	seedClient(t, ib, "buyer1@example.com", "11111111-1111-1111-1111-111111111111")

	links, err := svc.GetAllClientLinks("", "buyer1@example.com")
	if err != nil {
		t.Fatalf("GetAllClientLinks: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("no links built")
	}
	for _, l := range links {
		if !strings.Contains(l, "203.0.113.9") {
			t.Errorf("link does not name the inbound's address: %q", l)
		}
	}
}

// TestShopLinksFallBackToTheSubDomain: an inbound on a wildcard bind has no
// address of its own, so the panel's Sub domain fills in — which is exactly the
// setting the bot's removed "public address" field was duplicating.
func TestShopLinksFallBackToTheSubDomain(t *testing.T) {
	svc := withLinkProvider(t)
	if err := database.GetDB().Where(model.Setting{Key: "subDomain"}).
		Assign(model.Setting{Value: "sub.example.com"}).
		FirstOrCreate(&model.Setting{}).Error; err != nil {
		t.Fatalf("set subDomain: %v", err)
	}
	ib := &model.Inbound{
		UserId: 1, Tag: "shop-wildcard", Enable: true, Port: 43002, Protocol: model.VLESS,
		Listen: "", StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	seedClient(t, ib, "buyer2@example.com", "22222222-2222-2222-2222-222222222222")

	links, err := svc.GetAllClientLinks("", "buyer2@example.com")
	if err != nil {
		t.Fatalf("GetAllClientLinks: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("no links built")
	}
	for _, l := range links {
		if !strings.Contains(l, "sub.example.com") {
			t.Errorf("link did not fall back to the sub domain: %q", l)
		}
	}
}

// TestShopLinksAreAddresslessWhenNothingIsConfigured documents the one case the
// bot has to catch: no inbound address and no domain anywhere means the builder
// emits a link with nothing where the server should be.
func TestShopLinksAreAddresslessWhenNothingIsConfigured(t *testing.T) {
	svc := withLinkProvider(t)
	ib := &model.Inbound{
		UserId: 1, Tag: "shop-nothing", Enable: true, Port: 43003, Protocol: model.VLESS,
		Listen: "", StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	seedClient(t, ib, "buyer3@example.com", "33333333-3333-3333-3333-333333333333")

	links, err := svc.GetAllClientLinks("", "buyer3@example.com")
	if err != nil {
		t.Fatalf("GetAllClientLinks: %v", err)
	}
	for _, l := range links {
		if !strings.Contains(l, "@:") {
			t.Errorf("expected an address-less link for the bot to drop, got %q", l)
		}
	}
}
