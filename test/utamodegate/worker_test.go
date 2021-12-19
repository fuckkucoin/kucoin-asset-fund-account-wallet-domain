package utamodegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/utamodegate"
)

// TestWorker_BindExpiryReaper 验证过期绑定被正确标记为 expired，并对跨模式键触发漂移案。
func TestWorker_BindExpiryReaper(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-worker-reaper-01"
	// 先将世代推进至 1（切换至 CLASSIC），无在途负债
	_, err := svc.SetMode(ctx, userID, "CLASSIC", "test", "")
	if err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	// 一条在切换前（gen=0, UNIFIED 模式）注册但遗留至今的过期绑定
	// 直接写入 store，绕过 ValidateToken（世代已切换，直接历史遗留）
	now := time.Now().UTC()
	staleBind := &utamodegate.LiabilityBind{
		ID:		"bind-stale-01",
		UserID:		userID,
		BindKind:	utamodegate.BindKindBorrow,
		Currency:	"USDT",
		MarginMode:	utamodegate.MarginModeUnified,	// 旧模式键
		Amount:		"1000",
		Generation:	0,	// 旧世代
		RefID:		"borrow-stale-01",
		Status:		utamodegate.BindOpen,
		ExpiresAt:	now.Add(-1 * time.Hour),	// 已过期
		CreatedAt:	now.Add(-2 * time.Hour),
		UpdatedAt:	now.Add(-2 * time.Hour),
	}
	if err := svc.store.SaveBind(ctx, staleBind); err != nil {
		t.Fatalf("SaveBind (stale): %v", err)
	}
	// 执行 BindExpiryReaper
	w := utamodegate.NewWorker(svc, nil)
	expired, err := svc.workerMarkExpiredBinds(ctx, 100)
	if err != nil {
		t.Fatalf("workerMarkExpiredBinds: %v", err)
	}
	if len(expired) == 0 {
		t.Fatal("expected at least 1 expired bind")
	}
	// 验证过期的绑定在跨模式键场景下触发漂移案
	for _, b := range expired {
		mg, _ := svc.GetGeneration(ctx, b.UserID)
		currentMode := mg.CurrentMode
		if currentMode == "" {
			currentMode = utamodegate.MarginModeUnified
		}
		if b.MarginMode != currentMode {
			_ = svc.workerOpenDriftFromBind(ctx, b, currentMode)
		}
	}
	// 检查漂移案是否被开立（绑定 UNIFIED，当前 CLASSIC → 漂移）
	cases, err := svc.ListBlockingDriftCases(ctx, userID)
	if err != nil {
		t.Fatalf("ListBlockingDriftCases: %v", err)
	}
	if len(cases) == 0 {
		t.Error("expected drift case to be opened for stale-mode bind")
	}
	if len(cases) > 0 && cases[0].Kind != utamodegate.DriftKindLiabilityKey {
		t.Errorf("drift case kind = %s, want liability_key", cases[0].Kind)
	}
	_ = w	// suppress unused warning
}

// TestWorker_LiabilityScanner 验证在途负债清零后 blocked 租约被推进至 switched。
func TestWorker_LiabilityScanner(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-worker-scanner-01"
	// 注册在途负债（先有绑定）
	bind, err := svc.RegisterBind(ctx, utamodegate.RegisterBindReq{
		UserID:		userID,
		BindKind:	utamodegate.BindKindBorrow,
		Currency:	"ETH",
		MarginMode:	utamodegate.MarginModeCross,
		Amount:		"5",
		RefID:		"borrow-scan-01",
	}, 0)
	if err != nil {
		t.Fatalf("RegisterBind: %v", err)
	}
	// SetMode 进入 blocked 队列
	result, err := svc.SetMode(ctx, userID, "CLASSIC", "test", "")
	if err != utamodegate.ErrInflightLiability {
		t.Fatalf("SetMode: expected ErrInflightLiability, got %v", err)
	}
	leaseID := result.Lease.ID
	if result.Lease.Status != utamodegate.LeaseBlocked {
		t.Fatalf("lease status = %s, want blocked", result.Lease.Status)
	}
	// 释放绑定
	_, err = svc.ReleaseBind(ctx, utamodegate.ReleaseBindReq{UserID: userID, BindID: bind.ID})
	if err != nil {
		t.Fatalf("ReleaseBind: %v", err)
	}
	// worker 扫描推进
	switched, err := svc.workerUnblockLease(ctx, leaseID)
	if err != nil {
		t.Fatalf("workerUnblockLease: %v", err)
	}
	if switched.Status != utamodegate.LeaseSwitched {
		t.Errorf("switched.Status = %s, want switched", switched.Status)
	}
	// 世代应已递增
	gen, _ := svc.GetGeneration(ctx, userID)
	if gen.Generation < 1 {
		t.Errorf("generation = %d, want >= 1", gen.Generation)
	}
}

// TestWorker_DriftPatrol_Escalation 验证长时间未收口的漂移案触发告警。

func TestWorker_DriftPatrol_Escalation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-worker-patrol-01"
	dc, _ := svc.OpenDriftCase(ctx, utamodegate.OpenDriftCaseReq{
		UserID:			userID,
		Asset:			"USDC",
		Kind:			utamodegate.DriftKindCollateralKey,
		OldMode:		utamodegate.MarginModeUnified,
		NewMode:		utamodegate.MarginModeClassic,
		OldGeneration:		0,
		NewGeneration:		1,
		LiabilityAmount:	"9999",
	})
	// 将 DetectedAt 调整为超过 EscalationThreshold
	dc.DetectedAt = time.Now().UTC().Add(-20 * time.Minute)
	_ = svc.store.SaveDriftCase(ctx, dc)
	// 创建告警收集器
	events := make([]utamodegate.AlertEvent, 0)
	hook := &captureAlertHook{events: &events}
	w := utamodegate.NewWorker(svc, hook)
	w.escalateThreshold = 10 * time.Minute
	w.patrolDriftCases(ctx)
	// 应有 critical 告警
	found := false
	for _, e := range events {
		if e.Kind == "drift_case_escalate" && e.Severity == "critical" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected drift_case_escalate critical alert")
	}
}

// captureAlertHook 收集告警事件（用）。
type captureAlertHook struct {
	events *[]utamodegate.AlertEvent
}

func (h *captureAlertHook) Fire(_ context.Context, event utamodegate.AlertEvent,) error {
	*h.events = append(*h.events, event)
	return nil
}
