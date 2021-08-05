package keycer_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/keycer"
)

func TestRotateGenCounter_Monotonic(t *testing.T) {
	var c keycer.RotateGenCounter
	prev := int64(0)
	for i := 0; i < 100; i++ {
		next := c.Next()
		if next <= prev {
			t.Fatalf("not monotonic at i=%d: prev=%d next=%d", i, prev, next)
		}
		prev = next
	}
}

func TestRotateGenCounter_Current(t *testing.T) {
	var c keycer.RotateGenCounter
	if c.Current() != 0 {
		t.Errorf("initial current=%d, want 0", c.Current())
	}
	c.Next()
	if c.Current() != 1 {
		t.Errorf("after 1 Next, current=%d, want 1", c.Current())
	}
}

func TestSlotReaderAdapter_ReadSlot(t *testing.T) {
	svc := keycer.New()
	_ = svc
	ctx := context.Background()
	adapter := keycer.NewSlotReaderAdapter(svc)
	// slot-0 初始为 Empty，指纹为空，未退役
	fp, retired, err := adapter.ReadSlot(ctx, "slot-0")
	if err != nil {
		t.Fatal(err)
	}
	if retired {
		t.Error("empty slot should not be retired")
	}
	_ = fp
	// Load 槽后读取指纹
	_, err = svc.Load("slot-0", "test-label", "aabbccddeeff0011", "ops-alice")
	if err != nil {
		t.Fatal(err)
	}
	fp, retired, err = adapter.ReadSlot(ctx, "slot-0")
	if err != nil {
		t.Fatal(err)
	}
	if fp != "aabbccddeeff0011" {
		t.Errorf("fp=%q, want aabbccddeeff0011", fp)
	}
	if retired {
		t.Error("loaded slot should not be retired")
	}
}

func TestInflightGuard_SlotIsRetired(t *testing.T) {
	svc := keycer.New()
	g := keycer.NewInflightGuard(svc)
	// 初始 Empty：未退役
	retired, err := g.SlotIsRetired("slot-0")
	if err != nil {
		t.Fatal(err)
	}
	if retired {
		t.Error("empty slot should not be retired")
	}
	// 加载槽后 OpenRotate（slot-0 需处于 Loaded 或 Locked 状态）
	if _, err := svc.Load("slot-0", "lbl", "aabbccddeeff0011", "alice"); err != nil {
		t.Fatalf("Load slot-0: %v", err)
	}
	// slot-1 保持 Empty 也可触发轮换（dst 允许 Empty 或 Loaded）
	// 通过 ExecuteRotate 使 slot-0 退役
	tick, err := svc.OpenRotate("slot-0", "slot-1", "alice", "")
	if err != nil {
		t.Fatalf("OpenRotate: %v", err)
	}
	if _, err := svc.Approve(tick.ID, "bob"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := svc.ExecuteRotate(tick.ID, "charlie"); err != nil {
		t.Fatalf("ExecuteRotate: %v", err)
	}
	retired, err = g.SlotIsRetired("slot-0")
	if err != nil {
		t.Fatal(err)
	}
	if !retired {
		t.Error("slot-0 should be retired after ExecuteRotate")
	}
}
