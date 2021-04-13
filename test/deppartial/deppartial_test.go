package deppartial_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/deppartial"
)

// ──────────────────────────────────────────────────────────────
// 辅助：可控时钟
// ──────────────────────────────────────────────────────────────

type testClock struct {
	mu	sync.Mutex
	now	time.Time
}

func newTestClock(t time.Time) *testClock {
	return &testClock{now: t}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestService(clk *testClock, ttl time.Duration) *deppartial.Service {
	return deppartial.New(deppartial.WithClock(clk.Now), deppartial.WithLeaseTTL(ttl))
}

// ──────────────────────────────────────────────────────────────
// 表驱动：批次残余 FSM 边验证
// ──────────────────────────────────────────────────────────────

func TestBatchResidueFSMTransitions(t *testing.T) {
	m := deppartial.BatchResidueFSM()
	tests := []struct {
		name	string
		from	deppartial.BatchResidueState
		event	string
		wantTo	deppartial.BatchResidueState
		wantErr	bool
	}{
		{"submitted→residue_open", deppartial.BRSubmitted, deppartial.BrEvResidueFound, deppartial.BRResidueOpen, false},
		{"residue_open→reopening", deppartial.BRResidueOpen, deppartial.BrEvReopen, deppartial.BRReopening, false},
		{"reopening→closed", deppartial.BRReopening, deppartial.BrEvResolveDone, deppartial.BRClosed, false},
		{"reopening→residue_open", deppartial.BRReopening, deppartial.BrEvResidueFound, deppartial.BRResidueOpen, false},
		{"residue_open→closed(force)", deppartial.BRResidueOpen, deppartial.BrEvForceClose, deppartial.BRClosed, false},
		{"submitted→reopen illegal", deppartial.BRSubmitted, deppartial.BrEvReopen, deppartial.BRSubmitted, true},
		{"closed→anything illegal", deppartial.BRClosed, deppartial.BrEvResidueFound, deppartial.BRClosed, true},
		{"closed→reopen illegal", deppartial.BRClosed, deppartial.BrEvReopen, deppartial.BRClosed, true},
		{"submitted→resolve illegal", deppartial.BRSubmitted, deppartial.BrEvResolveDone, deppartial.BRSubmitted, true},
		{"submitted→force_close illegal", deppartial.BRSubmitted, deppartial.BrEvForceClose, deppartial.BRSubmitted, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：残余行 FSM 边验证
// ──────────────────────────────────────────────────────────────

func TestResidueFSMTransitions(t *testing.T) {
	m := deppartial.ResidueFSM()
	tests := []struct {
		name	string
		from	deppartial.ResidueState
		event	string
		wantTo	deppartial.ResidueState
		wantErr	bool
	}{
		{"open→credited", deppartial.ResOpen, deppartial.ResEvCreditRepair, deppartial.ResCredited, false},
		{"open→abandoned", deppartial.ResOpen, deppartial.ResEvAbandon, deppartial.ResAbandoned, false},
		{"open→escalated", deppartial.ResOpen, deppartial.ResEvEscalate, deppartial.ResEscalated, false},
		{"escalated→credited", deppartial.ResEscalated, deppartial.ResEvCreditRepair, deppartial.ResCredited, false},
		{"escalated→abandoned", deppartial.ResEscalated, deppartial.ResEvAbandon, deppartial.ResAbandoned, false},
		{"credited→anything illegal", deppartial.ResCredited, deppartial.ResEvAbandon, deppartial.ResCredited, true},
		{"abandoned→anything illegal", deppartial.ResAbandoned, deppartial.ResEvCreditRepair, deppartial.ResAbandoned, true},
		{"credited→escalate illegal", deppartial.ResCredited, deppartial.ResEvEscalate, deppartial.ResCredited, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：租约 FSM 边验证
// ──────────────────────────────────────────────────────────────

func TestLeaseFSMTransitions(t *testing.T) {
	m := deppartial.LeaseFSM()
	tests := []struct {
		name	string
		from	deppartial.LeaseStatus
		event	string
		wantTo	deppartial.LeaseStatus
		wantErr	bool
	}{
		{"held→released", deppartial.LeaseHeld, deppartial.LeaseEvRelease, deppartial.LeaseReleased, false},
		{"held→expired", deppartial.LeaseHeld, deppartial.LeaseEvExpire, deppartial.LeaseExpired, false},
		{"released→anything illegal", deppartial.LeaseReleased, deppartial.LeaseEvExpire, deppartial.LeaseReleased, true},
		{"expired→anything illegal", deppartial.LeaseExpired, deppartial.LeaseEvRelease, deppartial.LeaseExpired, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：一致证明 FSM 边验证
// ──────────────────────────────────────────────────────────────

func TestProofFSMTransitions(t *testing.T) {
	m := deppartial.ProofFSM()
	tests := []struct {
		name	string
		from	deppartial.ProofStatus
		event	string
		wantTo	deppartial.ProofStatus
		wantErr	bool
	}{
		{"pending→passed", deppartial.ProofPending, deppartial.ProofEvPass, deppartial.ProofPassed, false},
		{"pending→rejected", deppartial.ProofPending, deppartial.ProofEvReject, deppartial.ProofRejected, false},
		{"passed→anything illegal", deppartial.ProofPassed, deppartial.ProofEvReject, deppartial.ProofPassed, true},
		{"rejected→anything illegal", deppartial.ProofRejected, deppartial.ProofEvPass, deppartial.ProofRejected, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：残余扫描场景
// ──────────────────────────────────────────────────────────────

func TestScanResidues(t *testing.T) {
	tests := []struct {
		name		string
		items		[]deppartial.BatchItemSnapshot
		seen		[]deppartial.SeenRecord
		ledger		[]deppartial.LedgerRecord
		wantCount	int
		wantKinds	[]deppartial.ResidueKind
		wantBatchSt	deppartial.BatchResidueState
	}{
		{
			name:	"全部对齐无残余",
			items: []deppartial.BatchItemSnapshot{
				{Seq: 1, Chain: "ETH", TxHash: "0x111", Asset: "USDT", UserID: "U1", Status: "submitted", LedgerID: "L1"},
			},
			seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0x111", Occupied: true}},
			ledger:		[]deppartial.LedgerRecord{{Chain: "ETH", TxHash: "0x111", LedgerID: "L1", Amount: "100", Credited: true}},
			wantCount:	0,
			wantKinds:	nil,
			wantBatchSt:	deppartial.BRSubmitted,
		},
		{
			name:	"seen 占位无贷记",
			items: []deppartial.BatchItemSnapshot{
				{Seq: 1, Chain: "ETH", TxHash: "0x222", Asset: "USDT", UserID: "U2", Status: "submitted"},
			},
			seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0x222", Occupied: true}},
			ledger:		nil,
			wantCount:	1,
			wantKinds:	[]deppartial.ResidueKind{deppartial.KindSeenNoCredit},
			wantBatchSt:	deppartial.BRResidueOpen,
		},
		{
			name:	"贷记已入但 item 未提交",
			items: []deppartial.BatchItemSnapshot{
				{Seq: 1, Chain: "TRX", TxHash: "0x333", Asset: "USDT", UserID: "U3", Status: "pending"},
			},
			seen:		[]deppartial.SeenRecord{{Chain: "TRX", TxHash: "0x333", Occupied: true}},
			ledger:		[]deppartial.LedgerRecord{{Chain: "TRX", TxHash: "0x333", LedgerID: "L3", Amount: "50", Credited: true}},
			wantCount:	1,
			wantKinds:	[]deppartial.ResidueKind{deppartial.KindCreditNoItem},
			wantBatchSt:	deppartial.BRResidueOpen,
		},
		{
			name:	"多条混合残余",
			items: []deppartial.BatchItemSnapshot{
				{Seq: 1, Chain: "ETH", TxHash: "0x444", Asset: "USDT", UserID: "U4", Status: "submitted", LedgerID: "L4"},
				{Seq: 2, Chain: "ETH", TxHash: "0x555", Asset: "USDT", UserID: "U5", Status: "submitted"},
				{Seq: 3, Chain: "ETH", TxHash: "0x666", Asset: "ETH", UserID: "U6", Status: "pending"},
			},
			seen: []deppartial.SeenRecord{
				{Chain: "ETH", TxHash: "0x444", Occupied: true},
				{Chain: "ETH", TxHash: "0x555", Occupied: true},
				{Chain: "ETH", TxHash: "0x666", Occupied: true},
			},
			ledger: []deppartial.LedgerRecord{
				{Chain: "ETH", TxHash: "0x444", LedgerID: "L4", Amount: "100", Credited: true},
				{Chain: "ETH", TxHash: "0x666", LedgerID: "L6", Amount: "0.1", Credited: true},
			},
			wantCount:	2,
			wantKinds:	[]deppartial.ResidueKind{deppartial.KindSeenNoCredit, deppartial.KindCreditNoItem},
			wantBatchSt:	deppartial.BRResidueOpen,
		},
		{
			name:	"seen 未占位不产生残余",
			items: []deppartial.BatchItemSnapshot{
				{Seq: 1, Chain: "ETH", TxHash: "0x777", Asset: "USDT", UserID: "U7", Status: "submitted"},
			},
			seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0x777", Occupied: false}},
			ledger:		nil,
			wantCount:	0,
			wantKinds:	nil,
			wantBatchSt:	deppartial.BRSubmitted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2022,
				6, 15, 10, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 30*time.Second)
			batchID := "B-SCAN-" + tt.name
			if err := svc.RegisterBatch(batchID, len(tt.items)); err != nil {
				t.Fatal(err)
			}
			result, err := svc.Scan(deppartial.ScanInput{
				BatchID:	batchID,
				Items:		tt.items,
				Seen:		tt.seen,
				Ledger:		tt.ledger,
				Actor:		"test_scanner",
			})
			if err != nil {
				t.Fatalf("scan error: %v", err)
			}
			if result.ResidueCount != tt.wantCount {
				t.Fatalf("residue count: got %d, want %d", result.ResidueCount, tt.wantCount)
			}
			if tt.wantKinds != nil {
				gotKinds := map[deppartial.ResidueKind]bool{}
				for _, r := range result.Residues {
					gotKinds[r.Kind] = true
				}
				for _, wk := range tt.wantKinds {
					if !gotKinds[wk] {
						t.Fatalf("missing residue kind: %s", wk)
					}
				}
			}
			state, _ := svc.GetBatchState(batchID)
			if state != tt.wantBatchSt {
				t.Fatalf("batch state: got %s, want %s", state, tt.wantBatchSt)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：重开互斥与双人控制
// ──────────────────────────────────────────────────────────────

func TestAcquireLease(t *testing.T) {
	tests := []struct {
		name		string
		holder		string
		approver	string
		wantErr		error
	}{
		{"正常获取", "ops_A", "ops_B", nil},
		{"holder空", "", "ops_B", deppartial.ErrActor},
		{"approver空", "ops_A", "", deppartial.ErrDualControl},
		{"同一人", "ops_A", "ops_A", deppartial.ErrDualControl},
		{"空batch", "ops_A", "ops_B", deppartial.ErrBatchNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 30*time.Second)
			batchID := ""
			if tt.wantErr != deppartial.ErrBatchNotFound && tt.wantErr != deppartial.ErrActor && tt.wantErr != deppartial.ErrDualControl {
				batchID = "B-LEASE-" + tt.name
				_ = svc.RegisterBatch(batchID, 1)
				_, _ = svc.Scan(deppartial.ScanInput{
					BatchID:	batchID,
					Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xtest", Asset: "USDT", UserID: "U1", Status: "submitted"}},
					Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xtest", Occupied: true}},
					Actor:		"scanner",
				})
			} else if tt.wantErr == deppartial.ErrActor || tt.wantErr == deppartial.ErrDualControl {
				batchID = "B-LEASE-" + tt.name
				_ = svc.RegisterBatch(batchID, 1)
			}
			_, err := svc.AcquireLease(deppartial.ReopenReq{
				BatchID:	batchID,
				Holder:		tt.holder,
				Approver:	tt.approver,
				Reason:		"test",
			})
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if err != tt.wantErr {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：修复贷记与重复贷记封堵
// ──────────────────────────────────────────────────────────────

func TestRepairCreditAndDualBlock(t *testing.T) {
	tests := []struct {
		name	string
		setupFn	func(svc *deppartial.Service, batchID string) (residueID string, gen int64)
		repair	func(svc *deppartial.Service, batchID, residueID string, gen int64) error
		wantErr	error
	}{
		{
			name:	"正常修复",
			setupFn: func(svc *deppartial.Service, batchID string) (string, int64) {
				_ = svc.RegisterBatch(batchID, 1)
				result, _ := svc.Scan(deppartial.ScanInput{
					BatchID:	batchID,
					Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xR1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
					Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xR1", Occupied: true}},
					Actor:		"scanner",
				})
				lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops", Approver: "mgr", Reason: "fix"})
				return result.Residues[0].ID, lease.Generation
			},
			repair: func(svc *deppartial.Service, batchID, residueID string, gen int64) error {
				_, err := svc.RepairCredit(deppartial.RepairCreditReq{
					BatchID:	batchID, ResidueID: residueID, Actor: "ops",
					LedgerID:	"LED-NEW", Amount: "100", Generation: gen,
				})
				return err
			},
			wantErr:	nil,
		},
		{
			name:	"重复贷记封堵",
			setupFn: func(svc *deppartial.Service, batchID string) (string, int64) {
				_ = svc.RegisterBatch(batchID, 1)
				result, _ := svc.Scan(deppartial.ScanInput{
					BatchID:	batchID,
					Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xR2", Asset: "USDT", UserID: "U2", Status: "submitted"}},
					Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xR2", Occupied: true}},
					Actor:		"scanner",
				})
				lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops", Approver: "mgr", Reason: "fix"})
				svc.RepairCredit(deppartial.RepairCreditReq{
					BatchID:	batchID, ResidueID: result.Residues[0].ID, Actor: "ops",
					LedgerID:	"LED-FIRST", Amount: "100", Generation: lease.Generation,
				})
				return result.Residues[0].ID,
					lease.Generation
			},
			repair: func(svc *deppartial.Service, batchID, residueID string, gen int64) error {
				_, err := svc.RepairCredit(deppartial.RepairCreditReq{
					BatchID:	batchID, ResidueID: residueID, Actor: "ops",
					LedgerID:	"LED-SECOND", Amount: "100", Generation: gen,
				})
				return err
			},
			wantErr:	deppartial.ErrResidueNotOpen,
		},
		{
			name:	"无租约修复失败",
			setupFn: func(svc *deppartial.Service, batchID string) (string, int64) {
				_ = svc.RegisterBatch(batchID, 1)
				result, _ := svc.Scan(deppartial.ScanInput{
					BatchID:	batchID,
					Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xR3", Asset: "USDT", UserID: "U3", Status: "submitted"}},
					Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xR3", Occupied: true}},
					Actor:		"scanner",
				})
				return result.Residues[0].ID, 0
			},
			repair: func(svc *deppartial.Service, batchID, residueID string, gen int64) error {
				_, err := svc.RepairCredit(deppartial.RepairCreditReq{
					BatchID:	batchID, ResidueID: residueID, Actor: "ops",
					LedgerID:	"LED-FAIL", Amount: "100",
				})
				return err
			},
			wantErr:	deppartial.ErrLeaseNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
			svc := newTestService(clk,
				30*time.Second)
			batchID := "B-REPAIR-" + tt.name
			residueID, gen := tt.setupFn(svc, batchID)
			err := tt.repair(svc, batchID, residueID, gen)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if err != tt.wantErr {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 并发重开竞态
// ──────────────────────────────────────────────────────────────

func TestConcurrentReopen(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-CONCURRENT"
	_ = svc.RegisterBatch(batchID, 2)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items: []deppartial.BatchItemSnapshot{
			{Seq: 1, Chain: "ETH", TxHash: "0xCR1", Asset: "USDT", UserID: "U1", Status: "submitted"},
			{Seq: 2, Chain: "ETH", TxHash: "0xCR2", Asset: "USDT", UserID: "U2", Status: "submitted"},
		},
		Seen: []deppartial.SeenRecord{
			{Chain: "ETH", TxHash: "0xCR1", Occupied: true},
			{Chain: "ETH", TxHash: "0xCR2", Occupied: true},
		},
		Actor:	"scanner",
	})
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var errCount atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			holder := "ops_" + string(rune('A'+idx))
			approver := "mgr_" + string(rune('A'+idx))
			_, err := svc.AcquireLease(deppartial.ReopenReq{
				BatchID:	batchID,
				Holder:		holder,
				Approver:	approver,
				Reason:		"concurrent test",
			})
			if err == nil {
				successCount.Add(1)
			} else {
				errCount.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if successCount.Load() == 0 {
		t.Fatal("no goroutine acquired lease")
	}
	t.Logf("success=%d, blocked=%d", successCount.Load(), errCount.Load())
}

// ──────────────────────────────────────────────────────────────
// 同一 tx_hash 重复贷记封堵
// ──────────────────────────────────────────────────────────────

func TestDualCreditBlockSameTxHash(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-DUALCREDIT"
	_ = svc.RegisterBatch(batchID, 1)
	result, _ := svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xDC1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xDC1", Occupied: true}},
		Actor:		"scanner",
	})
	if len(result.Residues) == 0 {
		t.Fatal("expected residue")
	}
	lease, err := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops_A", Approver: "mgr_A", Reason: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RepairCredit(deppartial.RepairCreditReq{
		BatchID:	batchID, ResidueID: result.Residues[0].ID, Actor: "ops_A",
		LedgerID:	"LED-ONLY-ONE", Amount: "100", Generation: lease.Generation,
	})
	if err != nil {
		t.Fatalf("first repair should succeed: %v", err)
	}
	svc2 := svc
	_ = svc2.RegisterBatch("B-DUALCREDIT-2", 1)
	result2, _ := svc2.Scan(deppartial.ScanInput{
		BatchID:	"B-DUALCREDIT-2",
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xDC1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xDC1", Occupied: true}},
		Actor:		"scanner",
	})
	if len(result2.Residues) > 0 {
		lease2, _ := svc2.AcquireLease(deppartial.ReopenReq{BatchID: "B-DUALCREDIT-2", Holder: "ops_B", Approver: "mgr_B", Reason: "fix"})
		_, err = svc2.RepairCredit(deppartial.RepairCreditReq{
			BatchID:	"B-DUALCREDIT-2", ResidueID: result2.Residues[0].ID, Actor: "ops_B",
			LedgerID:	"LED-DUPLICATE", Amount: "100", Generation: lease2.Generation,
		})
		if err != deppartial.ErrDualCredit {
			t.Fatalf("expected ErrDualCredit, got %v", err)
		}
	}
}

// ──────────────────────────────────────────────────────────────
// 围栏关闭后阻止操作
// ──────────────────────────────────────────────────────────────

func TestFenceBlocksAfterClose(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-FENCE"
	_ = svc.RegisterBatch(batchID, 1)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xFN1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xFN1", Occupied: true}},
		Actor:		"scanner",
	})
	_, err := svc.CheckFence("ETH", "0xFN1")
	if err != nil {
		t.Fatalf("fence should be open: %v", err)
	}
	err = svc.MustNotFenced("ETH", "0xFN1")
	if err != nil {
		t.Fatalf("should not be fenced: %v", err)
	}
	lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops", Approver: "mgr", Reason: "fix"})
	residues, _ := svc.ListResidues(batchID)
	_, _ = svc.Abandon(deppartial.AbandonReq{
		BatchID:	batchID, ResidueID: residues[0].ID, Actor: "ops", Reason: "无法恢复",
	})
	_ = lease
	if !svc.IsFenceClosed("ETH", "0xFN1") {
		t.Fatal("fence should be closed after abandon")
	}
	_, err = svc.CheckFence("ETH", "0xFN1")
	if err != deppartial.ErrFenceClosed {
		t.Fatalf("expected ErrFenceClosed, got %v", err)
	}
	err = svc.MustNotFenced("ETH", "0xFN1")
	if err != deppartial.ErrFenceClosed {
		t.Fatalf("expected ErrFenceClosed, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────
// 关闭条件：需一致证明全部通过
// ──────────────────────────────────────────────────────────────

func TestCloseRequiresProof(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-CLOSE-PROOF"
	_ = svc.RegisterBatch(batchID, 2)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items: []deppartial.BatchItemSnapshot{
			{Seq: 1, Chain: "ETH", TxHash: "0xCP1", Asset: "USDT", UserID: "U1", Status: "submitted"},
			{Seq: 2, Chain: "ETH", TxHash: "0xCP2", Asset: "USDT", UserID: "U2", Status: "submitted"},
		},
		Seen: []deppartial.SeenRecord{
			{Chain: "ETH", TxHash: "0xCP1", Occupied: true},
			{Chain: "ETH", TxHash: "0xCP2", Occupied: true},
		},
		Actor:	"scanner",
	})
	lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops", Approver: "mgr", Reason: "fix"})
	residues, _ := svc.ListResidues(batchID)
	for _, r := range residues {
		_, err := svc.RepairCredit(deppartial.RepairCreditReq{
			BatchID:	batchID, ResidueID: r.ID, Actor: "ops",
			LedgerID:	"LED-" + r.TxHash, Amount: "100", Generation: lease.Generation,
		})
		if err != nil {
			t.Fatalf("repair error: %v", err)
		}
	}
	_, err := svc.CloseBatch(deppartial.CloseReq{BatchID: batchID, Actor: "closer"})
	if err != deppartial.ErrProofMissing {
		t.Fatalf("expected ErrProofMissing before align check, got %v", err)
	}
	_, err = svc.CheckAlignBatch(batchID, "auditor")
	if err != nil {
		t.Fatalf("align check error: %v", err)
	}
	view, err := svc.CloseBatch(deppartial.CloseReq{BatchID: batchID, Actor: "closer"})
	if err != nil {
		t.Fatalf("close error: %v", err)
	}
	if view.State != deppartial.BRClosed {
		t.Fatalf("expected closed, got %s", view.State)
	}
}

// ──────────────────────────────────────────────────────────────
// 租约过期与回收
// ──────────────────────────────────────────────────────────────

func TestLeaseExpiryAndReclaim(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 10*time.Second)
	batchID := "B-EXPIRE"
	_ = svc.RegisterBatch(batchID, 1)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xEX1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xEX1", Occupied: true}},
		Actor:		"scanner",
	})
	lease1, err := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops_A", Approver: "mgr_A", Reason: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops_B", Approver: "mgr_B", Reason: "compete"})
	if err != deppartial.ErrLeaseHeld {
		t.Fatalf("expected ErrLeaseHeld, got %v", err)
	}
	clk.Advance(20 * time.Second)
	expired := svc.ExpireLeases()
	if len(expired) == 0 {
		t.Fatal("expected expired lease")
	}
	if expired[0].ID != lease1.ID {
		t.Fatalf("wrong lease expired")
	}
	lease2, err := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops_B", Approver: "mgr_B", Reason: "takeover"})
	if err != nil {
		t.Fatalf("second acquire should succeed: %v", err)
	}
	if lease2.Holder != "ops_B" {
		t.Fatalf("wrong holder: %s", lease2.Holder)
	}
}

// ──────────────────────────────────────────────────────────────
// escalated 残余可被修复
// ──────────────────────────────────────────────────────────────

func TestEscalateThenRepair(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-ESCALATE"
	_ = svc.RegisterBatch(batchID, 1)
	result, _ := svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xES1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xES1", Occupied: true}},
		Actor:		"scanner",
	})
	_, err := svc.Escalate(deppartial.EscalateReq{
		BatchID:	batchID, ResidueID: result.Residues[0].ID, Actor: "tier1", Reason: "需人工",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := svc.GetResidue(result.Residues[0].ID)
	if r.State != deppartial.ResEscalated {
		t.Fatalf("expected escalated, got %s", r.State)
	}
	lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "tier2", Approver: "mgr", Reason: "fix"})
	repaired, err := svc.RepairCredit(deppartial.RepairCreditReq{
		BatchID:	batchID, ResidueID: result.Residues[0].ID, Actor: "tier2",
		LedgerID:	"LED-ESC", Amount: "500", Generation: lease.Generation,
	})
	if err != nil {
		t.Fatalf("repair after escalate should succeed: %v", err)
	}
	if repaired.State != deppartial.ResCredited {
		t.Fatalf("expected credited, got %s", repaired.State)
	}
}

// ──────────────────────────────────────────────────────────────
// 完整生命周期：扫描→重开→修复→一致→关闭
// ──────────────────────────────────────────────────────────────

func TestFullLifecycle(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk,
		30*time.Second)
	batchID := "B-LIFECYCLE"
	_ = svc.RegisterBatch(batchID, 3)
	result, err := svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items: []deppartial.BatchItemSnapshot{
			{Seq: 1, Chain: "ETH", TxHash: "0xLC1", Asset: "USDT", UserID: "U1", Status: "submitted", LedgerID: "L1"},
			{Seq: 2, Chain: "ETH", TxHash: "0xLC2", Asset: "USDT", UserID: "U2", Status: "submitted"},
			{Seq: 3, Chain: "ETH", TxHash: "0xLC3", Asset: "ETH", UserID: "U3", Status: "submitted"},
		},
		Seen: []deppartial.SeenRecord{
			{Chain: "ETH", TxHash: "0xLC1", Occupied: true},
			{Chain: "ETH", TxHash: "0xLC2", Occupied: true},
			{Chain: "ETH", TxHash: "0xLC3", Occupied: true},
		},
		Ledger: []deppartial.LedgerRecord{
			{Chain: "ETH", TxHash: "0xLC1", LedgerID: "L1", Amount: "100", Credited: true},
		},
		Actor:	"scanner",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResidueCount != 2 {
		t.Fatalf("expected 2 residues, got %d", result.ResidueCount)
	}
	state, _ := svc.GetBatchState(batchID)
	if state != deppartial.BRResidueOpen {
		t.Fatalf("expected residue_open, got %s", state)
	}
	can, reason, _ := svc.CanClose(batchID)
	if can {
		t.Fatal("should not be closable with open residues")
	}
	if reason != "open residues remain" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	lease, err := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops", Approver: "mgr", Reason: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	residues, _ := svc.ListResidues(batchID)
	_, err = svc.RepairCredit(deppartial.RepairCreditReq{
		BatchID:	batchID, ResidueID: residues[0].ID, Actor: "ops",
		LedgerID:	"LED-FIX-1", Amount: "200", Generation: lease.Generation,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Abandon(deppartial.AbandonReq{
		BatchID:	batchID, ResidueID: residues[1].ID, Actor: "ops", Reason: "不可恢复",
	})
	if err != nil {
		t.Fatal(err)
	}
	can, reason, _ = svc.CanClose(batchID)
	if can {
		t.Fatal("should not be closable without proofs")
	}
	if reason != "proof not all passed" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	proofs, err := svc.CheckAlignBatch(batchID, "auditor")
	if err != nil {
		t.Fatal(err)
	}
	allPassed := true
	for _, p := range proofs {
		if p.Status != deppartial.ProofPassed {
			allPassed = false
		}
	}
	if !allPassed {
		t.Fatal("expected all proofs passed")
	}
	view, err := svc.CloseBatch(deppartial.CloseReq{BatchID: batchID, Actor: "closer"})
	if err != nil {
		t.Fatal(err)
	}
	if view.State != deppartial.BRClosed {
		t.Fatalf("expected closed, got %s", view.State)
	}
	_, err = svc.CloseBatch(deppartial.CloseReq{BatchID: batchID, Actor: "closer"})
	if err != deppartial.ErrAlreadyClosed {
		t.Fatalf("expected ErrAlreadyClosed, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────
// 强制关闭
// ──────────────────────────────────────────────────────────────

func TestForceClose(t *testing.T) {
	tests := []struct {
		name		string
		actor		string
		approver	string
		wantErr		error
	}{
		{"正常强制关闭", "admin_A", "admin_B", nil},
		{"同一人拒绝", "admin_A", "admin_A", deppartial.ErrDualControl},
		{"缺approver", "admin_A", "", deppartial.ErrDualControl},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 30*time.Second)
			batchID := "B-FORCE-" + tt.name
			_ = svc.RegisterBatch(batchID, 1)
			_, _ = svc.Scan(deppartial.ScanInput{
				BatchID:	batchID,
				Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xFC1" + tt.name, Asset: "USDT", UserID: "U1", Status: "submitted"}},
				Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xFC1" + tt.name, Occupied: true}},
				Actor:		"scanner",
			})
			_, err := svc.ForceClose(batchID, tt.actor, tt.approver)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("expected %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 导出证明
// ──────────────────────────────────────────────────────────────

func TestExportProofs(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk,
		30*time.Second)
	batchID := "B-EXPORT"
	_ = svc.RegisterBatch(batchID, 2)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items: []deppartial.BatchItemSnapshot{
			{Seq: 1, Chain: "ETH", TxHash: "0xEP1", Asset: "USDT", UserID: "U1", Status: "submitted"},
			{Seq: 2, Chain: "ETH", TxHash: "0xEP2", Asset: "USDT", UserID: "U2", Status: "submitted"},
		},
		Seen: []deppartial.SeenRecord{
			{Chain: "ETH", TxHash: "0xEP1", Occupied: true},
			{Chain: "ETH", TxHash: "0xEP2", Occupied: true},
		},
		Actor:	"scanner",
	})
	lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops", Approver: "mgr", Reason: "fix"})
	residues,
		_ := svc.ListResidues(batchID)
	for _, r := range residues {
		svc.RepairCredit(deppartial.RepairCreditReq{
			BatchID:	batchID, ResidueID: r.ID, Actor: "ops",
			LedgerID:	"LED-" + r.ID, Amount: "100", Generation: lease.Generation,
		})
	}
	svc.CheckAlignBatch(batchID, "auditor")
	resp,
		err := svc.ExportProofs(deppartial.ProofExportReq{BatchID: batchID})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Summary.Total != 2 {
		t.Fatalf("expected 2 proofs, got %d", resp.Summary.Total)
	}
	if resp.Summary.Passed != 2 {
		t.Fatalf("expected 2 passed, got %d", resp.Summary.Passed)
	}
}

// ──────────────────────────────────────────────────────────────
// View 聚合视图
// ──────────────────────────────────────────────────────────────

func TestBatchView(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-VIEW"
	_ = svc.RegisterBatch(batchID, 3)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items: []deppartial.BatchItemSnapshot{
			{Seq: 1, Chain: "ETH", TxHash: "0xV1", Asset: "USDT", UserID: "U1", Status: "submitted"},
			{Seq: 2, Chain: "ETH", TxHash: "0xV2", Asset: "USDT", UserID: "U2", Status: "submitted"},
			{Seq: 3, Chain: "ETH", TxHash: "0xV3", Asset: "ETH", UserID: "U3", Status: "submitted", LedgerID: "L3"},
		},
		Seen: []deppartial.SeenRecord{
			{Chain: "ETH", TxHash: "0xV1", Occupied: true},
			{Chain: "ETH", TxHash: "0xV2", Occupied: true},
			{Chain: "ETH", TxHash: "0xV3", Occupied: true},
		},
		Ledger: []deppartial.LedgerRecord{
			{Chain: "ETH", TxHash: "0xV3", LedgerID: "L3", Amount: "1.5", Credited: true},
		},
		Actor:	"scanner",
	})
	view, err := svc.GetView(batchID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != deppartial.BRResidueOpen {
		t.Fatalf("expected residue_open, got %s", view.State)
	}
	if view.ResidueCount != 2 {
		t.Fatalf("expected 2 residues, got %d", view.ResidueCount)
	}
	if view.OpenCount != 2 {
		t.Fatalf("expected 2 open, got %d", view.OpenCount)
	}
	if view.TotalItems != 3 {
		t.Fatalf("expected 3 total items, got %d", view.TotalItems)
	}
	if view.CanClose {
		t.Fatal("should not be closable")
	}
}

// ──────────────────────────────────────────────────────────────
// 持久化往返
// ──────────────────────────────────────────────────────────────

func TestPersistRoundTrip(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-PERSIST"
	_ = svc.RegisterBatch(batchID, 2)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items: []deppartial.BatchItemSnapshot{
			{Seq: 1, Chain: "ETH", TxHash: "0xPR1", Asset: "USDT", UserID: "U1", Status: "submitted"},
			{Seq: 2, Chain: "ETH", TxHash: "0xPR2", Asset: "USDT", UserID: "U2", Status: "submitted"},
		},
		Seen: []deppartial.SeenRecord{
			{Chain: "ETH", TxHash: "0xPR1", Occupied: true},
			{Chain: "ETH", TxHash: "0xPR2", Occupied: true},
		},
		Actor:	"scanner",
	})
	ctx := context.Background()
	kv := persist.NewMemKV()
	if err := svc.Save(ctx, kv, "test-snap"); err != nil {
		t.Fatal(err)
	}
	loaded, err := deppartial.Load(ctx, kv, "test-snap")
	if err != nil {
		t.Fatal(err)
	}
	state,
		_ := loaded.GetBatchState(batchID)
	if state != deppartial.BRResidueOpen {
		t.Fatalf("expected residue_open after load, got %s", state)
	}
	residues, _ := loaded.ListResidues(batchID)
	if len(residues) != 2 {
		t.Fatalf("expected 2 residues after load, got %d", len(residues))
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：围栏操作
// ──────────────────────────────────────────────────────────────

func TestFenceOperations(t *testing.T) {
	tests := []struct {
		name		string
		chain		string
		txHash		string
		setFence	bool
		closeFence	bool
		wantCheck	error
		wantMust	error
	}{
		{"无围栏", "ETH", "0xNOFENCE", false, false, deppartial.ErrFenceNotFound, nil},
		{"开放围栏", "ETH", "0xOPEN", true, false, nil, nil},
		{"关闭围栏", "ETH", "0xCLOSED", true, true, deppartial.ErrFenceClosed, deppartial.ErrFenceClosed},
		{"空chain", "", "0xBAD", false, false, deppartial.ErrFenceNotFound, nil},
		{"空hash", "ETH", "", false, false, deppartial.ErrFenceNotFound, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := deppartial.New()
			if tt.setFence {
				svc.SetFence(tt.chain, tt.txHash, "B-TEST")
			}
			if tt.closeFence {
				svc.CloseFenceManual(tt.chain, tt.txHash)
			}
			_, err := svc.CheckFence(tt.chain, tt.txHash)
			if err != tt.wantCheck {
				t.Fatalf("CheckFence: expected %v, got %v", tt.wantCheck, err)
			}
			err = svc.MustNotFenced(tt.chain, tt.txHash)
			if err != tt.wantMust {
				t.Fatalf("MustNotFenced: expected %v, got %v", tt.wantMust, err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 批量扫描
// ──────────────────────────────────────────────────────────────

func TestScanAllBatches(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk,
		30*time.Second)
	_ = svc.RegisterBatch("B-MULTI-1", 1)
	_ = svc.RegisterBatch("B-MULTI-2", 1)
	inputs := []deppartial.ScanInput{
		{
			BatchID:	"B-MULTI-1",
			Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xM1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
			Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xM1", Occupied: true}},
			Actor:		"scanner",
		},
		{
			BatchID:	"B-MULTI-2",
			Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xM2", Asset: "USDT", UserID: "U2", Status: "submitted", LedgerID: "L2"}},
			Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xM2", Occupied: true}},
			Ledger:		[]deppartial.LedgerRecord{{Chain: "ETH", TxHash: "0xM2", LedgerID: "L2", Amount: "100", Credited: true}},
			Actor:		"scanner",
		},
	}
	found, err := svc.ScanAllBatches(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 batch with residues, got %d", len(found))
	}
	if found[0] != "B-MULTI-1" {
		t.Fatalf("expected B-MULTI-1, got %s", found[0])
	}
}

// ──────────────────────────────────────────────────────────────
// 释放租约
// ──────────────────────────────────────────────────────────────

func TestReleaseLease(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-RELEASE"
	_ = svc.RegisterBatch(batchID, 1)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xRL1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xRL1", Occupied: true}},
		Actor:		"scanner",
	})
	lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops_A", Approver: "mgr_A", Reason: "fix"})
	_, err := svc.ReleaseLease(lease.ID, "ops_B")
	if err != deppartial.ErrLeaseHeld {
		t.Fatalf("expected ErrLeaseHeld for wrong holder, got %v", err)
	}
	released, err := svc.ReleaseLease(lease.ID, "ops_A")
	if err != nil {
		t.Fatal(err)
	}
	if released.Status != deppartial.LeaseReleased {
		t.Fatalf("expected released, got %s", released.Status)
	}
	_, err = svc.GetActiveLease(batchID)
	if err != deppartial.ErrLeaseNotFound {
		t.Fatalf("expected ErrLeaseNotFound, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────
// 放弃残余后再修复应失败
// ──────────────────────────────────────────────────────────────

func TestAbandonThenRepairFails(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-ABANDON-REPAIR"
	_ = svc.RegisterBatch(batchID, 1)
	result, _ := svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xAR1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xAR1", Occupied: true}},
		Actor:		"scanner",
	})
	_, _ = svc.Abandon(deppartial.AbandonReq{BatchID: batchID, ResidueID: result.Residues[0].ID, Actor: "ops", Reason: "无法恢复"})
	lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops2", Approver: "mgr2", Reason: "retry"})
	_, err := svc.RepairCredit(deppartial.RepairCreditReq{
		BatchID:	batchID, ResidueID: result.Residues[0].ID, Actor: "ops2",
		LedgerID:	"LED-LATE", Amount: "100", Generation: lease.Generation,
	})
	if err != deppartial.ErrResidueNotOpen {
		t.Fatalf("expected ErrResidueNotOpen after abandon, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────
// 关闭后扫描被阻止
// ──────────────────────────────────────────────────────────────

func TestScanAfterCloseBlocked(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-CLOSED-SCAN"
	_ = svc.RegisterBatch(batchID,
		1)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xCS1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xCS1", Occupied: true}},
		Actor:		"scanner",
	})
	svc.ForceClose(batchID, "admin", "mgr")
	_, err := svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xCS2", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xCS2", Occupied: true}},
		Actor:		"scanner",
	})
	if err != deppartial.ErrAlreadyClosed {
		t.Fatalf("expected ErrAlreadyClosed, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────
// 工人
// ──────────────────────────────────────────────────────────────

func TestResidueScanWorker(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-WORKER"
	_ = svc.RegisterBatch(batchID, 1)
	inputFn := func(bid string) (deppartial.ScanInput, bool) {
		if bid != batchID {
			return deppartial.ScanInput{}, false
		}
		return deppartial.ScanInput{
			BatchID:	bid,
			Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xW1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
			Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xW1", Occupied: true}},
			Actor:		"worker_scanner",
		}, true
	}
	w := deppartial.NewResidueScanWorker(svc, time.Second, inputFn)
	w.RunOnce()
	state, _ := svc.GetBatchState(batchID)
	if state != deppartial.BRResidueOpen {
		t.Fatalf("expected residue_open after worker scan, got %s", state)
	}
}

func TestLeaseReclaimWorker(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 5*time.Second)
	batchID := "B-LEASE-WORKER"
	_ = svc.RegisterBatch(batchID, 1)
	_, _ = svc.Scan(deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xLW1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xLW1", Occupied: true}},
		Actor:		"scanner",
	})
	_, _ = svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops", Approver: "mgr", Reason: "fix"})
	clk.Advance(10 * time.Second)
	w := deppartial.NewLeaseReclaimWorker(svc, time.Second)
	w.RunOnce()
	_,
		err := svc.GetActiveLease(batchID)
	if err != deppartial.ErrLeaseNotFound {
		t.Fatalf("expected lease reclaimed, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：一致检查各种状态
// ──────────────────────────────────────────────────────────────

func TestAlignCheckVariousStates(t *testing.T) {
	tests := []struct {
		name		string
		residueSt	deppartial.ResidueState
		hasLedger	bool
		wantStatus	deppartial.ProofStatus
		wantAlign	bool
	}{
		{"credited有ledger", deppartial.ResCredited, true, deppartial.ProofPassed, true},
		{"abandoned", deppartial.ResAbandoned, false, deppartial.ProofPassed, true},
		{"open无ledger", deppartial.ResOpen, false, deppartial.ProofRejected, false},
		{"escalated无ledger", deppartial.ResEscalated, false, deppartial.ProofRejected, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 30*time.Second)
			batchID := "B-ALIGN-" + tt.name
			_ = svc.RegisterBatch(batchID, 1)
			result, _ := svc.Scan(deppartial.ScanInput{
				BatchID:	batchID,
				Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xAL-" + tt.name, Asset: "USDT", UserID: "U1", Status: "submitted"}},
				Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xAL-" + tt.name, Occupied: true}},
				Actor:		"scanner",
			})
			if len(result.Residues) > 0 {
				r := result.Residues[0]
				switch tt.residueSt {
				case deppartial.ResCredited:
					lease, _ := svc.AcquireLease(deppartial.ReopenReq{BatchID: batchID, Holder: "ops", Approver: "mgr", Reason: "fix"})
					svc.RepairCredit(deppartial.RepairCreditReq{
						BatchID:	batchID, ResidueID: r.ID, Actor: "ops",
						LedgerID:	"LED-TEST", Amount: "100", Generation: lease.Generation,
					})
				case deppartial.ResAbandoned:
					svc.Abandon(deppartial.AbandonReq{BatchID: batchID, ResidueID: r.ID, Actor: "ops", Reason: "test"})
				case deppartial.ResEscalated:
					svc.Escalate(deppartial.EscalateReq{BatchID: batchID, ResidueID: r.ID, Actor: "ops", Reason: "test"})
				}
			}
			proof, err := svc.CheckAlign(deppartial.AlignCheckReq{BatchID: batchID, Seq: 1, Checker: "auditor"})
			if err != nil {
				t.Fatal(err)
			}
			if proof.Status != tt.wantStatus {
				t.Fatalf("expected status %s, got %s", tt.wantStatus, proof.Status)
			}
			if proof.Aligned != tt.wantAlign {
				t.Fatalf("expected aligned=%v, got %v", tt.wantAlign, proof.Aligned)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// nil Service 安全性
// ──────────────────────────────────────────────────────────────

func TestNilServiceSafety(t *testing.T) {
	var svc *deppartial.Service
	if _, err := svc.Scan(deppartial.ScanInput{}); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.AcquireLease(deppartial.ReopenReq{}); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.RepairCredit(deppartial.RepairCreditReq{}); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.Abandon(deppartial.AbandonReq{}); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.Escalate(deppartial.EscalateReq{}); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.CheckAlign(deppartial.AlignCheckReq{}); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.CloseBatch(deppartial.CloseReq{}); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.GetView("x"); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.ListResidues("x"); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.ExportProofs(deppartial.ProofExportReq{}); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.CheckFence("a", "b"); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if err := svc.MustNotFenced("a", "b"); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.ReleaseLease("x", "y"); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.GetActiveLease("x"); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if err := svc.RegisterBatch("x", 1); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := svc.GetBatchState("x"); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if err := svc.Save(context.Background(), nil, "x"); err != deppartial.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────
// Golden file
// ──────────────────────────────────────────────────────────────

type goldenCase struct {
	Name		string	`json:"name"`
	Description	string	`json:"description"`
}

func TestGoldenFilesExist(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "deppartial")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot read testdata/deppartial: %v", err)
	}
	if len(entries) < 4 {
		t.Fatalf("expected at least 4 golden files, got %d", len(entries))
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("cannot read %s: %v", e.Name(), err)
		}
		var gc goldenCase
		if err := json.Unmarshal(data, &gc); err != nil {
			t.Fatalf("invalid JSON in %s: %v", e.Name(), err)
		}
		if gc.Name == "" {
			t.Fatalf("missing name in %s", e.Name())
		}
		if gc.Description == "" {
			t.Fatalf("missing description in %s", e.Name())
		}
		t.Logf("golden: %s — %s", gc.Name, gc.Description)
	}
}

// ──────────────────────────────────────────────────────────────
// 重复扫描幂等性
// ──────────────────────────────────────────────────────────────

func TestRescanIdempotent(t *testing.T) {
	clk := newTestClock(time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	batchID := "B-RESCAN"
	_ = svc.RegisterBatch(batchID, 1)
	input := deppartial.ScanInput{
		BatchID:	batchID,
		Items:		[]deppartial.BatchItemSnapshot{{Seq: 1, Chain: "ETH", TxHash: "0xRS1", Asset: "USDT", UserID: "U1", Status: "submitted"}},
		Seen:		[]deppartial.SeenRecord{{Chain: "ETH", TxHash: "0xRS1", Occupied: true}},
		Actor:		"scanner",
	}
	r1, _ := svc.Scan(input)
	r2, _ := svc.Scan(input)
	if r1.ResidueCount != 1 {
		t.Fatalf("expected 1 residue on first scan, got %d", r1.ResidueCount)
	}
	if r2.ResidueCount != 1 {
		t.Fatalf("expected 1 residue on second scan (new detection), got %d", r2.ResidueCount)
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：Store 操作
// ──────────────────────────────────────────────────────────────

func TestStoreBasicOps(t *testing.T) {
	st := deppartial.NewStore()
	st.RegisterBatch("B1", 3)
	s, ok := st.GetBatchState("B1")
	if !ok || s != deppartial.BRSubmitted {
		t.Fatal("batch not registered")
	}
	st.SetBatchState("B1", deppartial.BRResidueOpen)
	s2, _ := st.GetBatchState("B1")
	if s2 != deppartial.BRResidueOpen {
		t.Fatal("batch state not updated")
	}
	if st.GetBatchItems("B1") != 3 {
		t.Fatal("batch items wrong")
	}
	gen1 := st.NextGen()
	gen2 := st.NextGen()
	if gen2 <= gen1 {
		t.Fatal("generation not increasing")
	}
	all := st.AllBatchIDs()
	if len(all) != 1 || all[0] != "B1" {
		t.Fatal("AllBatchIDs wrong")
	}
	st.RegisterBatch("B2", 1)
	st.SetBatchState("B2", deppartial.BRClosed)
	open := st.BatchesInState(deppartial.BRResidueOpen)
	if len(open) != 1 || open[0] != "B1" {
		t.Fatal("BatchesInState wrong")
	}
}

func TestStoreCreditedHash(t *testing.T) {
	st := deppartial.NewStore()
	if !st.MarkCredited("ETH", "0x111") {
		t.Fatal("first credit should succeed")
	}
	if st.MarkCredited("ETH", "0x111") {
		t.Fatal("second credit should fail")
	}
	if !st.IsCredited("ETH", "0x111") {
		t.Fatal("should be credited")
	}
	if st.IsCredited("ETH", "0x222") {
		t.Fatal("should not be credited")
	}
}

func TestStoreExpiredLeases(t *testing.T) {
	st := deppartial.NewStore()
	now := time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC)
	l1 := &deppartial.ReopenLease{ID: "L1", BatchID: "B1", Status: deppartial.LeaseHeld, ExpiresAt: now.Add(-time.Minute)}
	l2 := &deppartial.ReopenLease{ID: "L2", BatchID: "B2", Status: deppartial.LeaseHeld, ExpiresAt: now.Add(time.Minute)}
	l3 := &deppartial.ReopenLease{ID: "L3", BatchID: "B3", Status: deppartial.LeaseReleased, ExpiresAt: now.Add(-time.Minute)}
	st.PutLease(l1)
	st.PutLease(l2)
	st.PutLease(l3)
	expired := st.ExpiredLeases(now)
	if len(expired) != 1 || expired[0].ID != "L1" {
		t.Fatalf("expected 1 expired lease (L1), got %d", len(expired))
	}
}
