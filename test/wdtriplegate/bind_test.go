package wdtriplegate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/wdtriplegate"
)

func newGateWithClock(t *testing.T) (*wdtriplegate.Gate, func() interface{}) {
	t.Helper()
	now := testNow
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(now))
	return g, func() interface{} { return nil }
}

// TestBindNoDrift 无漂移主路径：BroadcastAllowed=true，租约推进 Bound。
func TestBindNoDrift(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	lease, err := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-bind-01", UserID: "u-1", TicketClear: true,
		KYTPin:	"allow", ListGen: 10, PolicyVer: 3, IssuedBy: "ops",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	result, err := g.Bind(wdtriplegate.BindReq{
		MovementID:		"mv-bind-01",
		TripleLeaseID:		lease.TripleLeaseID,
		CurrentListGen:		10,	// 与租约快照一致
		CurrentPolicyVer:	3,
		CurrentKYTPin:		"allow",
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if !result.BroadcastAllowed {
		t.Error("BroadcastAllowed should be true when no drift")
	}
	if result.DriftDetected {
		t.Error("DriftDetected should be false")
	}
	// 验证租约已推进到 Bound
	updated, _ := g.engine.GetLease(lease.TripleLeaseID)
	if updated.State != wdtriplegate.LeaseBound {
		t.Errorf("lease state: %q want bound", updated.State)
	}
}

// TestBindListGenDrift list_gen 漂移场景：BroadcastAllowed=false，租约 Blocked，DriftCase 产出。
func TestBindListGenDrift(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-bind-drift-01", UserID: "u-1", TicketClear: true,
		KYTPin:	"allow", ListGen: 10, PolicyVer: 3, IssuedBy: "ops",
	})
	result, err := g.Bind(wdtriplegate.BindReq{
		MovementID:		"mv-bind-drift-01",
		TripleLeaseID:		lease.TripleLeaseID,
		CurrentListGen:		11,	// 漂移：list_gen 从 10 变成 11
		CurrentPolicyVer:	3,
		CurrentKYTPin:		"allow",
	})
	if err != wdtriplegate.ErrDriftDetected {
		t.Errorf("err: %v want ErrDriftDetected", err)
	}
	if result.BroadcastAllowed {
		t.Error("BroadcastAllowed should be false on drift")
	}
	if !result.DriftDetected {
		t.Error("DriftDetected should be true")
	}
	if result.DriftCaseID == "" {
		t.Error("DriftCaseID should be set")
	}
	// 验证租约已 Blocked
	updated, _ := g.engine.GetLease(lease.TripleLeaseID)
	if updated.State != wdtriplegate.LeaseBlocked {
		t.Errorf("lease state: %q want blocked", updated.State)
	}
	// 验证 DriftCase 已产出
	dc,
		err := g.GetDriftCase(result.DriftCaseID)
	if err != nil {
		t.Fatalf("get drift case: %v", err)
	}
	if dc.DriftKind != wdtriplegate.DriftListGen {
		t.Errorf("drift kind: %q want list_gen", dc.DriftKind)
	}
	if dc.LeaseListGen != 10 || dc.CurrentListGen != 11 {
		t.Errorf("gen mismatch: lease=%d current=%d", dc.LeaseListGen, dc.CurrentListGen)
	}
}

// TestBindPolicyVerDrift policy_ver 漂移场景。
func TestBindPolicyVerDrift(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-bind-drift-02", UserID: "u-2", TicketClear: true,
		KYTPin:	"allow", ListGen: 20, PolicyVer: 5, IssuedBy: "ops",
	})
	result, err := g.Bind(wdtriplegate.BindReq{
		MovementID:		"mv-bind-drift-02",
		TripleLeaseID:		lease.TripleLeaseID,
		CurrentListGen:		20,
		CurrentPolicyVer:	6,	// 漂移：policy_ver 从 5 变成 6
		CurrentKYTPin:		"allow",
	})
	if err != wdtriplegate.ErrDriftDetected {
		t.Errorf("err: %v want ErrDriftDetected", err)
	}
	if result.BroadcastAllowed {
		t.Error("BroadcastAllowed should be false")
	}
	dc, _ := g.GetDriftCase(result.DriftCaseID)
	if dc.DriftKind != wdtriplegate.DriftPolicyVer {
		t.Errorf("drift kind: %q want policy_ver", dc.DriftKind)
	}
}

// TestBindKYTPinDrift KYT 钉撤销漂移场景（kyt_pin 变为 review）。

func TestBindKYTPinDrift(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-bind-drift-03", UserID: "u-3", TicketClear: true,
		KYTPin:	"allow", ListGen: 30, PolicyVer: 8, IssuedBy: "ops",
	})
	result, err := g.Bind(wdtriplegate.BindReq{
		MovementID:		"mv-bind-drift-03",
		TripleLeaseID:		lease.TripleLeaseID,
		CurrentListGen:		30,
		CurrentPolicyVer:	8,
		CurrentKYTPin:		"review",	// KYT 钉漂移
	})
	if err != wdtriplegate.ErrDriftDetected {
		t.Errorf("err: %v want ErrDriftDetected", err)
	}
	dc, _ := g.GetDriftCase(result.DriftCaseID)
	if dc.DriftKind != wdtriplegate.DriftKYTPin {
		t.Errorf("drift kind: %q want kyt_pin", dc.DriftKind)
	}
}

// TestBindIdempotent 重复调用 Bind 幂等（返回相同绑定结果）。
func TestBindIdempotent(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-bind-idem", UserID: "u-4", TicketClear: true,
		KYTPin:	"allow", ListGen: 5, PolicyVer: 1, IssuedBy: "ops",
	})
	r1, err1 := g.Bind(wdtriplegate.BindReq{
		MovementID:	"mv-bind-idem", TripleLeaseID: lease.TripleLeaseID,
		CurrentListGen:	5, CurrentPolicyVer: 1, CurrentKYTPin: "allow",
	})
	if err1 != nil {
		t.Fatalf("first bind: %v", err1)
	}
	r2, err2 := g.Bind(wdtriplegate.BindReq{
		MovementID:	"mv-bind-idem", TripleLeaseID: lease.TripleLeaseID,
		CurrentListGen:	5, CurrentPolicyVer: 1, CurrentKYTPin: "allow",
	})
	if err2 != nil {
		t.Fatalf("second bind: %v", err2)
	}
	if r1.BindID != r2.BindID {
		t.Errorf("bind id mismatch: %q != %q (not idempotent)", r1.BindID, r2.BindID)
	}
}

// TestRequireBroadcastAllowed 广播口校验。
func TestRequireBroadcastAllowed(t *testing.T) {
	g := wdtriplegate.NewGate(wdtriplegate.GateConfig{Store: wdtriplegate.NewMemStore()}).WithClock(fixedClock(testNow))
	// 未绑定时拒绝
	if err := g.RequireBroadcastAllowed("mv-no-bind"); err != wdtriplegate.ErrBroadcastBlocked {
		t.Errorf("no bind: got %v want ErrBroadcastBlocked", err)
	}
	lease, _ := g.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-bc-01", UserID: "u-5", TicketClear: true,
		KYTPin:	"allow", ListGen: 1, PolicyVer: 1, IssuedBy: "ops",
	})
	g.Bind(wdtriplegate.BindReq{
		MovementID:	"mv-bc-01", TripleLeaseID: lease.TripleLeaseID,
		CurrentListGen:	1, CurrentPolicyVer: 1, CurrentKYTPin: "allow",
	})
	if err := g.RequireBroadcastAllowed("mv-bc-01"); err != nil {
		t.Errorf("after valid bind: got %v want nil", err)
	}
}
