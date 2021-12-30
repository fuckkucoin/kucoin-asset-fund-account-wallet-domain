package wdtriplegate_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdtriplegate"
)

// TestMemStoreLeaseConflict SaveLease 同 movement_id 活跃租约冲突检测。
func TestMemStoreLeaseConflict(t *testing.T) {
	s := wdtriplegate.NewMemStore()
	now := testNow
	lease := wdtriplegate.TripleLease{
		TripleLeaseID:	"tl-ms-01", MovementID: "mv-ms-01",
		State:	wdtriplegate.LeaseIssued, IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute),
		CreatedAt:	now, UpdatedAt: now,
	}
	if err := s.SaveLease(lease); err != nil {
		t.Fatalf("first save: %v", err)
	}
	lease2 := lease
	lease2.TripleLeaseID = "tl-ms-02"
	if err := s.SaveLease(lease2); err != wdtriplegate.ErrBindConflict {
		t.Errorf("duplicate: got %v want ErrBindConflict", err)
	}
}

// TestMemStoreCAS UpdateLeaseStateCAS 乐观锁。
func TestMemStoreCAS(t *testing.T) {
	s := wdtriplegate.NewMemStore()
	now := testNow
	lease := wdtriplegate.TripleLease{
		TripleLeaseID:	"tl-cas-01", MovementID: "mv-cas-01",
		State:	wdtriplegate.LeaseIssued, IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute),
		CreatedAt:	now, UpdatedAt: now,
	}
	_ = s.SaveLease(lease)
	// 合法：Issued→Bound
	bound, err := s.UpdateLeaseStateCAS("tl-cas-01", wdtriplegate.LeaseIssued, wdtriplegate.LeaseBound, now, nil)
	if err != nil || bound.State != wdtriplegate.LeaseBound {
		t.Errorf("CAS issued→bound: err=%v state=%q", err, bound.State)
	}
	// 非法：从 Bound 再次推进 Issued→Bound（from 不匹配）
	_, err = s.UpdateLeaseStateCAS("tl-cas-01", wdtriplegate.LeaseIssued, wdtriplegate.LeaseBound, now, nil)
	if err != wdtriplegate.ErrInvalidTransition {
		t.Errorf("re-CAS: got %v want ErrInvalidTransition", err)
	}
}

// TestMemStoreBindIdempotent AppendBind 幂等性。
func TestMemStoreBindIdempotent(t *testing.T) {
	s := wdtriplegate.NewMemStore()
	now := testNow
	bind := wdtriplegate.PreflightBind{
		BindID:	"bi-01", MovementID: "mv-bind-01",
		TripleLeaseID:	"tl-01", ListGenAtBind: 1, PolicyVerAtBind: 1,
		BroadcastAllowed:	true, BoundAt: now, CreatedAt: now,
	}
	_ = s.AppendBind(bind)
	_ = s.AppendBind(bind)	// 第二次写入应幂等忽略
	got, err := s.GetBindByMovement("mv-bind-01")
	if err != nil {
		t.Fatalf("get bind: %v", err)
	}
	if got.BindID != "bi-01" {
		t.Errorf("bind id: %q want bi-01", got.BindID)
	}
}

// TestMemStoreDriftCaseIdempotent AppendDriftCase 幂等性。
func TestMemStoreDriftCaseIdempotent(t *testing.T) {
	s := wdtriplegate.NewMemStore()
	now := testNow
	dc := wdtriplegate.TripleDriftCase{
		DriftCaseID:	"dc-01", MovementID: "mv-dc-01",
		TripleLeaseID:	"tl-01", DriftKind: wdtriplegate.DriftListGen,
		LeaseListGen:	5, CurrentListGen: 6,
		State:	wdtriplegate.DriftOpen, DetectedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	_ = s.AppendDriftCase(dc)
	_ = s.AppendDriftCase(dc)	// 幂等
	got, err := s.GetDriftCase("dc-01")
	if err != nil {
		t.Fatalf("get drift case: %v", err)
	}
	if got.DriftCaseID != "dc-01" {
		t.Errorf("id: %q want dc-01", got.DriftCaseID)
	}
}

// TestMemStoreDriftCaseCASTransition DriftCase 状态转换 CAS 。

func TestMemStoreDriftCaseCASTransition(t *testing.T) {
	s := wdtriplegate.NewMemStore()
	now := testNow
	dc := wdtriplegate.TripleDriftCase{
		DriftCaseID:	"dc-cas-01", MovementID: "mv-cas-dc",
		DriftKind:	wdtriplegate.DriftPolicyVer,
		State:		wdtriplegate.DriftOpen,
		DetectedAt:	now, CreatedAt: now, UpdatedAt: now,
	}
	_ = s.AppendDriftCase(dc)
	proven, err := s.UpdateDriftCaseState("dc-cas-01", wdtriplegate.DriftOpen, wdtriplegate.DriftProven, now,
		&wdtriplegate.DriftCaseExtra{ProofChecksum: "abc123"})
	if err != nil || proven.State != wdtriplegate.DriftProven {
		t.Errorf("Open→Proven: err=%v state=%q", err, proven.State)
	}
	closed, err := s.UpdateDriftCaseState("dc-cas-01", wdtriplegate.DriftProven, wdtriplegate.DriftClosed, now,
		&wdtriplegate.DriftCaseExtra{ConfirmedBy: "alice", CoConfirmedBy: "bob"})
	if err != nil || closed.State != wdtriplegate.DriftClosed {
		t.Errorf("Proven→Closed: err=%v state=%q", err, closed.State)
	}
	// 终态不可再转换
	_, err = s.UpdateDriftCaseState("dc-cas-01", wdtriplegate.DriftClosed, wdtriplegate.DriftOpen, now, nil)
	if err != wdtriplegate.ErrInvalidTransition {
		t.Errorf("Closed→Open: got %v want ErrInvalidTransition", err)
	}
}
