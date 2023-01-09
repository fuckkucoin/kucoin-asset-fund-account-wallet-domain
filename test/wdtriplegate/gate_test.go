package wdtriplegate_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdtriplegate"
)

// TestGateFullHappyPath 无漂移完整正常路径。

func TestGateFullHappyPath(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	// 1. IssueLease（Preflight 成功后调用）
	lease, err := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-gate-hp", UserID: "u-gate", Chain: "ETH",
		Dest:	"0xGATEHAPPY", TicketClear: true,
		KYTPin:	"allow", ListGen: 100, PolicyVer: 10, IssuedBy: "ops",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// 2. Bind（广播前复验）
	result, err := g.Bind(wdtriplegate.BindReq{
		MovementID:		"mv-gate-hp",
		TripleLeaseID:		lease.TripleLeaseID,
		CurrentListGen:		100,
		CurrentPolicyVer:	10,
		CurrentKYTPin:		"allow",
	})
	if err != nil || !result.BroadcastAllowed {
		t.Fatalf("bind: err=%v allowed=%v", err, result.BroadcastAllowed)
	}
	// 3. RequireBroadcastAllowed（广播口最终校验）
	if err := g.RequireBroadcastAllowed("mv-gate-hp"); err != nil {
		t.Errorf("broadcast check: %v", err)
	}
	// 4. ConsumeLease（广播成功后收口）
	consumed, err := g.ConsumeLease(lease.TripleLeaseID)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if consumed.State != wdtriplegate.LeaseConsumed {
		t.Errorf("state: %q want consumed", consumed.State)
	}
}

// TestGateDriftRaceCondition 预检后 list_gen 抬升竞态：广播前复验应拒绝。
func TestGateDriftRaceCondition(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-gate-race", UserID: "u-gate", TicketClear: true,
		KYTPin:	"allow", ListGen: 50, PolicyVer: 5, IssuedBy: "ops",
	})
	// 预检后白名单世代抬升（list_gen: 50 → 51）
	result, err := g.Bind(wdtriplegate.BindReq{
		MovementID:		"mv-gate-race",
		TripleLeaseID:		lease.TripleLeaseID,
		CurrentListGen:		51,	// 竞态！
		CurrentPolicyVer:	5,
		CurrentKYTPin:		"allow",
	})
	if err != wdtriplegate.ErrDriftDetected {
		t.Errorf("err: %v want ErrDriftDetected", err)
	}
	if result.BroadcastAllowed {
		t.Error("BroadcastAllowed must be false after drift")
	}
	// 广播口必须拒绝
	if err := g.RequireBroadcastAllowed("mv-gate-race"); err != wdtriplegate.ErrBroadcastBlocked {
		t.Errorf("broadcast check after drift: %v want ErrBroadcastBlocked", err)
	}
}

// TestGateLeaseExpiredRecheck 租约过期后广播口必须拒绝，要求重检。

func TestGateLeaseExpiredRecheck(t *testing.T) {
	issueTime := testNow
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(issueTime))
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-gate-exp", UserID: "u-gate", TicketClear: true,
		KYTPin:	"allow", ListGen: 1, PolicyVer: 1, IssuedBy: "ops", TTLSeconds: 10,
	})
	// 强制过期租约
	g.WithClock(fixedClock(issueTime.Add(20 * time.Second)))
	g.ForceExpire(lease.TripleLeaseID)
	// 验证租约已 Expired
	updated, _ := g.engine.GetLease(lease.TripleLeaseID)
	if updated == nil || updated.State != wdtriplegate.LeaseExpired {
		t.Fatalf("lease state: %v want expired", updated)
	}
	// RequireBroadcastAllowed 未绑定时拒绝
	if err := g.RequireBroadcastAllowed("mv-gate-exp"); err != wdtriplegate.ErrBroadcastBlocked {
		t.Errorf("after expire: got %v want ErrBroadcastBlocked", err)
	}
}

// TestGateLeaseStatus 租约状态聚合查询。

func TestGateLeaseStatus(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-gate-status", UserID: "u-gs", TicketClear: true,
		KYTPin:	"allow", ListGen: 7, PolicyVer: 2, IssuedBy: "ops",
	})
	// 无绑定时查询
	resp, err := g.LeaseStatus("mv-gate-status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if resp.Lease == nil || resp.Lease.TripleLeaseID != lease.TripleLeaseID {
		t.Error("lease not found in status")
	}
	if resp.Bind != nil {
		t.Error("bind should be nil before Bind() call")
	}
	// 绑定后查询
	g.Bind(wdtriplegate.BindReq{
		MovementID:		"mv-gate-status",
		TripleLeaseID:		lease.TripleLeaseID,
		CurrentListGen:		7,
		CurrentPolicyVer:	2,
		CurrentKYTPin:		"allow",
	})
	resp2, _ := g.LeaseStatus("mv-gate-status")
	if resp2.Bind == nil {
		t.Error("bind should be present after Bind() call")
	}
	if !resp2.Bind.BroadcastAllowed {
		t.Error("bind should allow broadcast")
	}
}

// TestGateDriftCaseConfirm 漂移案件完整确认流程。

func TestGateDriftCaseConfirm(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-gate-dc", UserID: "u-gc", TicketClear: true,
		KYTPin:	"allow", ListGen: 99, PolicyVer: 9, IssuedBy: "ops",
	})
	result, _ := g.Bind(wdtriplegate.BindReq{
		MovementID:		"mv-gate-dc",
		TripleLeaseID:		lease.TripleLeaseID,
		CurrentListGen:		100,	// 漂移
		CurrentPolicyVer:	9,
		CurrentKYTPin:		"allow",
	})
	// 附上证明
	dc, err := g.AttachDriftProof(result.DriftCaseID, "sha256-proof-999")
	if err != nil {
		t.Fatalf("attach proof: %v", err)
	}
	if dc.State != wdtriplegate.DriftProven {
		t.Errorf("state: %q want proven", dc.State)
	}
	// 双人确认关闭
	closed, err := g.ConfirmDriftClose(wdtriplegate.ConfirmDriftCaseReq{
		DriftCaseID:	result.DriftCaseID,
		ConfirmedBy:	"ops-alice",
		CoConfirmedBy:	"ops-bob",
		Note:		"no double spend; re-check issued",
	})
	if err != nil {
		t.Fatalf("confirm close: %v", err)
	}
	if closed.State != wdtriplegate.DriftClosed {
		t.Errorf("state: %q want closed", closed.State)
	}
	if closed.ConfirmedBy != "ops-alice" || closed.CoConfirmedBy != "ops-bob" {
		t.Error("confirmed_by mismatch")
	}
}
