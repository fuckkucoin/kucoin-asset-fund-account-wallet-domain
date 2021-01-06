package subaccounts_test

import (
	"testing"

	"account-assets.kucoin.net/funding/internal/subaccounts"
)

func TestNormalizeLifecycle(t *testing.T) {
	ev := subaccounts.NormalizeLifecycle(subaccounts.LifecycleEvent{
		EventType: "created", MasterUID: "m1", SubUserID: "s1",
	})
	if ev.EventType != "CREATED" || ev.EventID == "" || ev.OccurredAt == "" {
		t.Fatalf("%+v", ev)
	}
}
