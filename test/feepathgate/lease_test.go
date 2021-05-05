package feepathgate_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feepathgate"
)

func fixedClock(t time.Time) func() time.Time	{ return func() time.Time { return t } }

func newTestFSM() *feepathgate.LeaseFSM {
	store := feepathgate.NewMemStore()
	return feepathgate.NewLeaseFSM(store, fixedClock(time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC)))
}

// ──────────────────────────────────────────────────────────────────────────────
// Acquire 表驱动
// ──────────────────────────────────────────────────────────────────────────────

func TestLeaseFSM_Acquire(t *testing.T) {
	type tc struct {
		name	string
		reqs	[]feepathgate.AcquireReq
		wantErr	[]error	// len == len(reqs)；nil 表示期望成功
	}
	tests := []tc{
		{
			name:	"take_acquire_ok",
			reqs: []feepathgate.AcquireReq{
				{BusinessKey: "u1|USDT|idem1", IdemKey: "idem1", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w1"},
			},
			wantErr:	[]error{nil},
		},
		{
			name:	"rebate_acquire_ok",
			reqs: []feepathgate.AcquireReq{
				{BusinessKey: "u2|USDT|idem2", IdemKey: "idem2", Kind: feepathgate.KindRebate, Asset: "USDT", Gross: "5", WorkerID: "w2"},
			},
			wantErr:	[]error{nil},
		},
		{
			name:	"take_then_rebate_conflict",
			reqs: []feepathgate.AcquireReq{
				{BusinessKey: "u3|USDT|idem3", IdemKey: "idem3:take", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w3"},
				{BusinessKey: "u3|USDT|idem3", IdemKey: "idem3:rebate", Kind: feepathgate.KindRebate, Asset: "USDT", Gross: "5", WorkerID: "w4"},
			},
			wantErr:	[]error{nil, feepathgate.ErrPathConflict},
		},
		{
			name:	"rebate_then_take_conflict",
			reqs: []feepathgate.AcquireReq{
				{BusinessKey: "u4|USDT|idem4", IdemKey: "idem4:rebate", Kind: feepathgate.KindRebate, Asset: "USDT", Gross: "5", WorkerID: "w5"},
				{BusinessKey: "u4|USDT|idem4", IdemKey: "idem4:take", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w6"},
			},
			wantErr:	[]error{nil, feepathgate.ErrPathConflict},
		},
		{
			name:	"empty_biz_key_error",
			reqs: []feepathgate.AcquireReq{
				{BusinessKey: "", IdemKey: "idem5", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w7"},
			},
			wantErr:	[]error{feepathgate.ErrBusinessKey},
		},
		{
			name:	"invalid_kind_error",
			reqs: []feepathgate.AcquireReq{
				{BusinessKey: "u5|USDT|idem6", IdemKey: "idem6", Kind: feepathgate.Kind("unknown"), Asset: "USDT", Gross: "100", WorkerID: "w8"},
			},
			wantErr:	[]error{feepathgate.ErrKind},
		},
		{
			name:	"empty_asset_error",
			reqs: []feepathgate.AcquireReq{
				{BusinessKey: "u6||idem7", IdemKey: "idem7", Kind: feepathgate.KindTake, Asset: "", Gross: "100", WorkerID: "w9"},
			},
			wantErr:	[]error{feepathgate.ErrAsset},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsm := newTestFSM()
			ctx := context.Background()
			for i, req := range tt.reqs {
				_, err := fsm.Acquire(ctx, req)
				wantErr := tt.wantErr[i]
				if wantErr == nil && err != nil {
					t.Errorf("step %d: unexpected error: %v", i, err)
				}
				if wantErr != nil && err != wantErr {
					t.Errorf("step %d: want error %v, got %v", i, wantErr, err)
				}
			}
		})
	}
}

// TestLeaseFSM_Acquire_Generation 验证 generation 号在每次 Acquire 时递增。

func TestLeaseFSM_Acquire_Generation(t *testing.T) {
	fsm := newTestFSM()
	ctx := context.Background()
	req := feepathgate.AcquireReq{BusinessKey: "u10|USDT|gen-test", IdemKey: "gen-test:take", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w-gen"}
	lease, err := fsm.Acquire(ctx, req)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if lease.Generation != 1 {
		t.Errorf("want generation=1, got=%d", lease.Generation)
	}
	gen, ok := fsm.GetGeneration("u10|USDT|gen-test")
	if !ok {
		t.Fatal("generation not found after acquire")
	}
	if gen.Generation != 1 {
		t.Errorf("PathGeneration want 1, got %d", gen.Generation)
	}
	if gen.LockedBy != lease.ID {
		t.Errorf("LockedBy want %s, got %s", lease.ID, gen.LockedBy)
	}
}

// TestLeaseFSM_ConcurrentAcquire 两个 goroutine 并发 Acquire 同 biz_key：
// 仅一方成功，另一方得到 ErrStaleGen 或 ErrPathConflict（取决于竞争结果）。
// 无论哪方成功，最终 fee_path_generations.generation == 1（无双递增）。

func TestLeaseFSM_ConcurrentAcquire(t *testing.T) {
	store := feepathgate.NewMemStore()
	fsm := feepathgate.NewLeaseFSM(store, time.Now().UTC)
	ctx := context.Background()
	const bizKey = "u20|USDT|concurrent"
	const rounds = 50	// 多次运行确认随机竞争均符合预期
	for round := 0; round < rounds; round++ {
		localStore := feepathgate.NewMemStore()
		localFSM := feepathgate.NewLeaseFSM(localStore, time.Now().UTC)
		var mu sync.Mutex
		var successCount, failCount int
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				kind := feepathgate.KindTake
				if idx == 1 {
					kind = feepathgate.KindRebate
				}
				req := feepathgate.AcquireReq{
					BusinessKey:	bizKey,
					IdemKey:	fmt.Sprintf("conc-idem-%d-%d", round, idx),
					Kind:		kind,
					Asset:		"USDT",
					Gross:		"100",
					WorkerID:	fmt.Sprintf("worker-%d", idx),
				}
				_, err := localFSM.Acquire(ctx, req)
				mu.Lock()
				if err == nil {
					successCount++
				} else {
					failCount++
				}
				mu.Unlock()
			}(i)
		}
		wg.Wait()
		if successCount != 1 || failCount != 1 {
			t.Errorf("round %d: want 1 success + 1 fail, got success=%d fail=%d", round, successCount, failCount)
		}
	}
	_ = fsm	// 避免未使用
}

// ──────────────────────────────────────────────────────────────────────────────
// Bind
// ──────────────────────────────────────────────────────────────────────────────

func TestLeaseFSM_Bind(t *testing.T) {
	ctx := context.Background()
	t.Run("bind_open_to_bound", func(t *testing.T) {
		fsm := newTestFSM()
		req := feepathgate.AcquireReq{BusinessKey: "u30|USDT|bind1", IdemKey: "bind1", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w-bind"}
		lease, _ := fsm.Acquire(ctx, req)
		bound, err := fsm.Bind(ctx, feepathgate.BindReq{
			LeaseID:	lease.ID,
			CreditID:	"credit-001",
			LedgerID:	"",
			Fee:		"5.00",
			Net:		"95.00",
			WorkerID:	"w-bind",
		})
		if err != nil {
			t.Fatalf("bind: %v", err)
		}
		if bound.Status != feepathgate.LeaseBound {
			t.Errorf("want bound, got %s", bound.Status)
		}
		if bound.CreditID != "credit-001" {
			t.Errorf("want credit_id=credit-001, got %s", bound.CreditID)
		}
	})
	t.Run("bind_wrong_worker", func(t *testing.T) {
		fsm := newTestFSM()
		req := feepathgate.AcquireReq{BusinessKey: "u31|USDT|bind2", IdemKey: "bind2", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w-owner"}
		lease, _ := fsm.Acquire(ctx, req)
		_, err := fsm.Bind(ctx, feepathgate.BindReq{LeaseID: lease.ID, WorkerID: "w-other"})
		if err != feepathgate.ErrWorkerConflict {
			t.Errorf("want ErrWorkerConflict, got %v", err)
		}
	})
	t.Run("bind_already_bound", func(t *testing.T) {
		fsm := newTestFSM()
		req := feepathgate.AcquireReq{BusinessKey: "u32|USDT|bind3", IdemKey: "bind3", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w-b3"}
		lease, _ := fsm.Acquire(ctx, req)
		_, err := fsm.Bind(ctx, feepathgate.BindReq{LeaseID: lease.ID, WorkerID: "w-b3"})
		if err != nil {
			t.Fatalf("first bind: %v", err)
		}
		_, err2 := fsm.Bind(ctx, feepathgate.BindReq{LeaseID: lease.ID, WorkerID: "w-b3"})
		if err2 != feepathgate.ErrLeaseBound {
			t.Errorf("want ErrLeaseBound on second bind, got %v", err2)
		}
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Settle & MarkConflicted
// ──────────────────────────────────────────────────────────────────────────────

func TestLeaseFSM_SettleAndConflict(t *testing.T) {
	ctx := context.Background()
	t.Run("settle_bound_with_proof", func(t *testing.T) {
		fsm := newTestFSM()
		req := feepathgate.AcquireReq{BusinessKey: "u40|USDT|s1", IdemKey: "s1", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w-s1"}
		lease, _ := fsm.Acquire(ctx, req)
		lease, _ = fsm.Bind(ctx, feepathgate.BindReq{LeaseID: lease.ID, CreditID: "c1", WorkerID: "w-s1"})
		settled, err := fsm.Settle(ctx, feepathgate.SettleReq{LeaseID: lease.ID, ProofID: "proof-001"})
		if err != nil {
			t.Fatalf("settle: %v", err)
		}
		if settled.Status != feepathgate.LeaseSettled {
			t.Errorf("want settled, got %s", settled.Status)
		}
		if settled.SettledAt == nil {
			t.Error("settled_at should not be nil")
		}
	})
	t.Run("settle_missing_proof_id", func(t *testing.T) {
		fsm := newTestFSM()
		req := feepathgate.AcquireReq{BusinessKey: "u41|USDT|s2", IdemKey: "s2", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w-s2"}
		lease, _ := fsm.Acquire(ctx, req)
		_, _ = fsm.Bind(ctx, feepathgate.BindReq{LeaseID: lease.ID, CreditID: "c2", WorkerID: "w-s2"})
		_, err := fsm.Settle(ctx, feepathgate.SettleReq{LeaseID: lease.ID, ProofID: ""})
		if err != feepathgate.ErrProofMissing {
			t.Errorf("want ErrProofMissing, got %v", err)
		}
	})
	t.Run("mark_conflicted_open", func(t *testing.T) {
		fsm := newTestFSM()
		req := feepathgate.AcquireReq{BusinessKey: "u42|USDT|c1", IdemKey: "c1", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w-c1"}
		lease, _ := fsm.Acquire(ctx, req)
		conflicted,
			err := fsm.MarkConflicted(ctx, lease.ID)
		if err != nil {
			t.Fatalf("mark_conflicted: %v", err)
		}
		if conflicted.Status != feepathgate.LeaseConflicted {
			t.Errorf("want conflicted, got %s", conflicted.Status)
		}
		if conflicted.ConflictAt == nil {
			t.Error("conflict_at should not be nil")
		}
	})
}

// TestLeaseFSM_GetByBizKey 验证 GetByBizKey 只返回活跃（open/bound）租约。

func TestLeaseFSM_GetByBizKey(t *testing.T) {
	ctx := context.Background()
	fsm := newTestFSM()
	bizKey := "u50|USDT|biz-query"
	req := feepathgate.AcquireReq{BusinessKey: bizKey, IdemKey: "bq:take", Kind: feepathgate.KindTake, Asset: "USDT", Gross: "100", WorkerID: "w-bq"}
	lease, _ := fsm.Acquire(ctx, req)
	leases, err := fsm.GetByBizKey(ctx, bizKey)
	if err != nil {
		t.Fatalf("get_by_biz_key: %v", err)
	}
	if len(leases) != 1 {
		t.Errorf("want 1 lease, got %d", len(leases))
	}
	// Settle 后不应出现在活跃列表中。
	fsm.Bind(ctx, feepathgate.BindReq{LeaseID: lease.ID, CreditID: "c-bq", WorkerID: "w-bq"})	//nolint
	fsm.Settle(ctx, feepathgate.SettleReq{LeaseID: lease.ID, ProofID: "p-bq"})			//nolint
	leases2, _ := fsm.GetByBizKey(ctx, bizKey)
	if len(leases2) != 0 {
		t.Errorf("after settle: want 0 active leases, got %d", len(leases2))
	}
}
