package wdtriplegate_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdtriplegate"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

var testNow = time.Date(2025, 11, 28, 10, 0, 0, 0, time.UTC)

// TestIssueLease IssueLease 主路径与校验失败。
func TestIssueLease(t *testing.T) {
	tests := []struct {
		name	string
		req	wdtriplegate.IssueLeaseReq
		wantErr	error
	}{
		{
			name:	"valid issue",
			req: wdtriplegate.IssueLeaseReq{
				MovementID:	"mv-0001", UserID: "u-1", Chain: "ETH", Dest: "0xABCD",
				TicketClear:	true, KYTPin: "allow", ListGen: 1, PolicyVer: 1, IssuedBy: "ops",
			},
			wantErr:	nil,
		},
		{
			name:	"open ticket blocks issue",
			req: wdtriplegate.IssueLeaseReq{
				MovementID:	"mv-0002", TicketClear: false,
				KYTPin:	"allow", ListGen: 1, PolicyVer: 1, IssuedBy: "ops",
			},
			wantErr:	wdtriplegate.ErrTicketOpen,
		},
		{
			name:	"invalid kyt pin",
			req: wdtriplegate.IssueLeaseReq{
				MovementID:	"mv-0003", TicketClear: true,
				KYTPin:	"deny", ListGen: 1, PolicyVer: 1, IssuedBy: "ops",
			},
			wantErr:	wdtriplegate.ErrInvalidKYTPin,
		},
		{
			name:	"zero list gen",
			req: wdtriplegate.IssueLeaseReq{
				MovementID:	"mv-0004", TicketClear: true,
				KYTPin:	"allow", ListGen: 0, PolicyVer: 1, IssuedBy: "ops",
			},
			wantErr:	wdtriplegate.ErrInvalidListGen,
		},
		{
			name:	"zero policy ver",
			req: wdtriplegate.IssueLeaseReq{
				MovementID:	"mv-0005", TicketClear: true,
				KYTPin:	"allow", ListGen: 1, PolicyVer: 0, IssuedBy: "ops",
			},
			wantErr:	wdtriplegate.ErrInvalidPolicyVer,
		},
		{
			name:	"empty movement id",
			req: wdtriplegate.IssueLeaseReq{
				MovementID:	"", TicketClear: true,
				KYTPin:	"allow", ListGen: 1, PolicyVer: 1, IssuedBy: "ops",
			},
			wantErr:	wdtriplegate.ErrMovementRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := wdtriplegate.NewEngine(wdtriplegate.NewMemStore()).WithClock(fixedClock(testNow))
			_, err := e.IssueLease(tt.req)
			if err != tt.wantErr {
				t.Errorf("IssueLease() err=%v want=%v", err, tt.wantErr)
			}
		})
	}
}

// TestDuplicateLeaseConflict 同 movement_id 重复签发应返回 ErrBindConflict。
func TestDuplicateLeaseConflict(t *testing.T) {
	store := wdtriplegate.NewMemStore()
	e := wdtriplegate.NewEngine(store).WithClock(fixedClock(testNow))
	req := wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-dup", UserID: "u-1", TicketClear: true,
		KYTPin:	"allow", ListGen: 1, PolicyVer: 1, IssuedBy: "ops",
	}
	if _, err := e.IssueLease(req); err != nil {
		t.Fatalf("first issue failed: %v", err)
	}
	_, err := e.IssueLease(req)
	if err != wdtriplegate.ErrBindConflict {
		t.Errorf("second issue: got %v want ErrBindConflict", err)
	}
}

// TestLeaseLifecycle Issued→Bound→Consumed 正常生命周期。
func TestLeaseLifecycle(t *testing.T) {
	store := wdtriplegate.NewMemStore()
	clock := testNow
	e := wdtriplegate.NewEngine(store).WithClock(fixedClock(clock))
	lease, err := e.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-lc-01", UserID: "u-1", TicketClear: true,
		KYTPin:	"allow", ListGen: 5, PolicyVer: 2, IssuedBy: "ops",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if lease.State != wdtriplegate.LeaseIssued {
		t.Errorf("state after issue: %q want issued", lease.State)
	}
	// Bind
	bound, err := e.BindLease(lease.TripleLeaseID)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if bound.State != wdtriplegate.LeaseBound {
		t.Errorf("state after bind: %q want bound", bound.State)
	}
	// Consume
	consumed, err := e.ConsumeLease(lease.TripleLeaseID)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if consumed.State != wdtriplegate.LeaseConsumed {
		t.Errorf("state after consume: %q want consumed", consumed.State)
	}
	// 二次 consume 返回 ErrLeaseTerminal
	if _, err := e.ConsumeLease(lease.TripleLeaseID); err != wdtriplegate.ErrLeaseTerminal {
		t.Errorf("re-consume: got %v want ErrLeaseTerminal", err)
	}
}

// TestLeaseExpiredByTTL TTL 过期后 BindLease 应自动推进 Expired。

func TestLeaseExpiredByTTL(t *testing.T) {
	store := wdtriplegate.NewMemStore()
	issueTime := testNow
	e := wdtriplegate.NewEngine(store).WithClock(fixedClock(issueTime))
	lease, err := e.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-ttl-01", UserID: "u-1", TicketClear: true,
		KYTPin:	"allow", ListGen: 1, PolicyVer: 1, IssuedBy: "ops", TTLSeconds: 5,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// 推进时钟超过 TTL
	e.WithClock(fixedClock(issueTime.Add(10 * time.Second)))
	_, err = e.BindLease(lease.TripleLeaseID)
	if err != wdtriplegate.ErrLeaseExpired {
		t.Errorf("bind after TTL: got %v want ErrLeaseExpired", err)
	}
	// 确认租约已变 Expired
	updated, _ := e.GetLease(lease.TripleLeaseID)
	if updated.State != wdtriplegate.LeaseExpired {
		t.Errorf("state after TTL bind: %q want expired", updated.State)
	}
}

// TestBlockLease BlockLease 正常工作并写入 DriftCaseID。
func TestBlockLease(t *testing.T) {
	store := wdtriplegate.NewMemStore()
	e := wdtriplegate.NewEngine(store).WithClock(fixedClock(testNow))
	lease,
		_ := e.IssueLease(wdtriplegate.IssueLeaseReq{
		MovementID:	"mv-block-01", UserID: "u-1", TicketClear: true,
		KYTPin:	"allow", ListGen: 10, PolicyVer: 3, IssuedBy: "ops",
	})
	blocked, err := e.BlockLease(lease.TripleLeaseID, "dc-999", "drift detected")
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if blocked.State != wdtriplegate.LeaseBlocked {
		t.Errorf("state: %q want blocked", blocked.State)
	}
	if blocked.DriftCaseID != "dc-999" {
		t.Errorf("drift_case_id: %q want dc-999", blocked.DriftCaseID)
	}
	// 二次 block 返回 ErrLeaseTerminal
	if _, err := e.BlockLease(lease.TripleLeaseID, "dc-000", "re-block"); err != wdtriplegate.ErrLeaseTerminal {
		t.Errorf("re-block: got %v want ErrLeaseTerminal", err)
	}
}
