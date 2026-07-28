package job

import (
	"fmt"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func setBillingInterval(t *testing.T, minutes int) {
	t.Helper()
	if err := database.GetDB().Where(model.Setting{Key: "shopBillingInterval"}).
		Assign(model.Setting{Value: fmt.Sprintf("%d", minutes)}).
		FirstOrCreate(&model.Setting{}).Error; err != nil {
		t.Fatalf("set interval: %v", err)
	}
}

// TestBillingHonoursTheConfiguredInterval: the cron ticks every minute and the
// job decides whether its own interval has elapsed, so an admin changing the
// setting does not have to restart the panel.
func TestBillingHonoursTheConfiguredInterval(t *testing.T) {
	setupJobDB(t)
	setBillingInterval(t, 5)
	j := NewShopBillingJob(nil, nil)

	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if !j.due(start) {
		t.Fatal("the first tick after start must bill rather than wait out the interval")
	}
	for _, after := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute} {
		if j.due(start.Add(after)) {
			t.Errorf("billed %v into a 5 minute interval", after)
		}
	}
	if !j.due(start.Add(5 * time.Minute)) {
		t.Error("the interval elapsed but the job did not bill")
	}
	// The clock restarts from the run that just happened, not from the first one.
	if j.due(start.Add(6 * time.Minute)) {
		t.Error("billed again one minute after a run")
	}
	if !j.due(start.Add(10 * time.Minute)) {
		t.Error("the second interval elapsed but the job did not bill")
	}

	// Shortening the interval applies to the very next tick.
	setBillingInterval(t, 1)
	if !j.due(start.Add(11 * time.Minute)) {
		t.Error("a shortened interval did not take effect")
	}
}

// TestBillingIntervalCannotBeSwitchedOff: a zero, a negative or a wild number
// must not turn metering off or push it a week out — that would hand out free
// traffic. Out-of-range values fall back inside the supported bounds.
func TestBillingIntervalCannotBeSwitchedOff(t *testing.T) {
	setupJobDB(t)
	settings := service.SettingService{}

	for _, tc := range []struct {
		stored, want int
	}{
		{0, service.ShopBillingIntervalDefault},
		{-30, service.ShopBillingIntervalDefault},
		{1, 1},
		{60, 60},
		{1440, 1440},
		{100000, service.ShopBillingIntervalMax},
	} {
		setBillingInterval(t, tc.stored)
		got, err := settings.GetShopBillingInterval()
		if err != nil {
			t.Fatalf("GetShopBillingInterval(%d): %v", tc.stored, err)
		}
		if got != tc.want {
			t.Errorf("stored %d, got %d minutes, want %d", tc.stored, got, tc.want)
		}
	}

	// A zero in the settings must not make every tick bill.
	setBillingInterval(t, 0)
	j := NewShopBillingJob(nil, nil)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if !j.due(start) {
		t.Fatal("first tick should bill")
	}
	if j.due(start.Add(time.Minute)) {
		t.Error("a stored 0 was treated as 'bill on every tick' rather than the default")
	}
	if !j.due(start.Add(time.Duration(service.ShopBillingIntervalDefault) * time.Minute)) {
		t.Error("the default interval did not elapse")
	}
}
