package lendoccupygate_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/lendoccupygate"
)

// stubLending 可控错误的 LendingService 桩。
type stubLending struct {
	mu			sync.Mutex
	placeErr		error	// 注入 Place 错误（借记失败）
	holdErr			error	// 注入 hold 失败（半截）
	settleErr		error
	cancelErr		error
	compensateErr		error
	placeCalled		int64	// 原子计数
	settleCalled		int64
	cancelCalled		int64
	compensateCalled	int64
	creditLog		[]string	// 记录每次贷记操作（供双贷记断言）
}

func (s *stubLending) Place(_ context.Context, userID, asset, amount, orderID string) (string, string, error) {
	atomic.AddInt64(&s.placeCalled, 1)
	if s.placeErr != nil {
		return "", "", s.placeErr
	}
	// holdErr 借记成功但 hold 失败
	if s.holdErr != nil {
		return "", "lend-ord:" + orderID, s.holdErr
	}
	return orderID, "lend-ord:" + orderID, nil
}

func (s *stubLending) Settle(_ context.Context, userID, asset, amount, orderID string) error {
	atomic.AddInt64(&s.settleCalled, 1)
	s.mu.Lock()
	s.creditLog = append(s.creditLog, fmt.Sprintf("settle:%s:%s:%s", userID, asset, orderID))
	s.mu.Unlock()
	return s.settleErr
}

func (s *stubLending) Cancel(_ context.Context, userID, asset, amount, orderID string) error {
	atomic.AddInt64(&s.cancelCalled, 1)
	s.mu.Lock()
	s.creditLog = append(s.creditLog, fmt.Sprintf("cancel:%s:%s:%s", userID, asset, orderID))
	s.mu.Unlock()
	return s.cancelErr
}

func (s *stubLending) CompensateDebit(_ context.Context, userID, asset, amount, orderID string) error {
	atomic.AddInt64(&s.compensateCalled, 1)
	s.mu.Lock()
	s.creditLog = append(s.creditLog, fmt.Sprintf("compensate:%s:%s:%s", userID, asset, orderID))
	s.mu.Unlock()
	return s.compensateErr
}

func newTestEngine(lend lendoccupygate.LendingService,) (*lendoccupygate.Engine, *lendoccupygate.MemStore,) {
	store := lendoccupygate.NewMemStore()
	engine := lendoccupygate.NewEngine(store, lend, lendoccupygate.WithEngineClock(func() time.Time {
		return time.Date(2025,
			12, 1, 12, 0, 0, 0, time.UTC)
	}))
	return engine, store
}

// ── PlaceBound ──────────────────────────────────────────────────────────

func TestEngine_PlaceBound(t *testing.T) {
	cases := []struct {
		name		string
		req		lendoccupygate.PlaceBoundReq
		injectErr	error
		wantErr		bool
		wantErrIs	error
		checkResp	func(*testing.T, *lendoccupygate.PlaceBoundResp,)
	}{
		{
			name:	"正常挂单签发 gen 和 lease",
			req: lendoccupygate.PlaceBoundReq{
				OrderID:	"ord-001",
				UserID:		"user-001",
				Asset:		"USDT",
				Amount:		"100",
			},
			wantErr:	false,
			checkResp: func(t *testing.T, resp *lendoccupygate.PlaceBoundResp,) {
				if resp.Lease.State != lendoccupygate.LeasePlaced {
					t.Errorf("lease state = %s, want placed", resp.Lease.State)
				}
				if resp.Gen.Consumed {
					t.Error("gen should not be consumed after place")
				}
				if resp.Lease.GenID != resp.Gen.GenID {
					t.Error("lease.GenID != gen.GenID")
				}
				if resp.Lease.OrderID != "ord-001" {
					t.Errorf("lease.OrderID = %s, want ord-001", resp.Lease.OrderID)
				}
			},
		},
		{
			name:		"缺少 OrderID 返回 ErrOrderRequired",
			req:		lendoccupygate.PlaceBoundReq{UserID: "u", Asset: "USDT", Amount: "1"},
			wantErr:	true,
			wantErrIs:	lendoccupygate.ErrOrderRequired,
		},
		{
			name:		"金额为零返回 ErrAmountInvalid",
			req:		lendoccupygate.PlaceBoundReq{OrderID: "o", UserID: "u", Asset: "USDT", Amount: "0"},
			wantErr:	true,
			wantErrIs:	lendoccupygate.ErrAmountInvalid,
		},
		{
			name:		"金额为负数返回 ErrAmountInvalid",
			req:		lendoccupygate.PlaceBoundReq{OrderID: "o", UserID: "u", Asset: "USDT", Amount: "-1"},
			wantErr:	true,
			wantErrIs:	lendoccupygate.ErrAmountInvalid,
		},
		{
			name:		"缺少 UserID 返回 ErrUserRequired",
			req:		lendoccupygate.PlaceBoundReq{OrderID: "o", Asset: "USDT", Amount: "1"},
			wantErr:	true,
			wantErrIs:	lendoccupygate.ErrUserRequired,
		},
		{
			name:		"缺少 Asset 返回 ErrAssetRequired",
			req:		lendoccupygate.PlaceBoundReq{OrderID: "o", UserID: "u", Amount: "1"},
			wantErr:	true,
			wantErrIs:	lendoccupygate.ErrAssetRequired,
		},
		{
			name:		"lending.Place 失败直接返回",
			req:		lendoccupygate.PlaceBoundReq{OrderID: "o-err", UserID: "u", Asset: "USDT", Amount: "10"},
			injectErr:	errors.New("ledger unavailable"),
			wantErr:	true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubLending{placeErr: tc.injectErr}
			engine, _ := newTestEngine(stub)
			resp, err := engine.PlaceBound(context.Background(), tc.req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Errorf("error = %v, want %v", err, tc.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.checkResp != nil {
				tc.checkResp(t, resp)
			}
		})
	}
}

// ── PlaceBound 幂等 ───────────────────────────────────────────────────────

func TestEngine_PlaceBound_Idempotent(t *testing.T) {
	stub := &stubLending{}
	engine, _ := newTestEngine(stub)
	req := lendoccupygate.PlaceBoundReq{OrderID: "ord-idem-001", UserID: "user-idem", Asset: "BTC", Amount: "0.5"}
	resp1, err := engine.PlaceBound(context.Background(), req)
	if err != nil {
		t.Fatalf("first PlaceBound failed: %v", err)
	}
	// 同 order 重复调用应返回 ErrGenAlreadyExists
	_, err = engine.PlaceBound(context.Background(), req)
	if !errors.Is(err, lendoccupygate.ErrGenAlreadyExists) {
		t.Errorf("second PlaceBound: got %v, want ErrGenAlreadyExists", err)
	}
	_ = resp1
}

// ── Settle ───────────────────────────────────────────────────────────────

func TestEngine_Settle(t *testing.T) {
	cases := []struct {
		name		string
		setup		func(*lendoccupygate.Engine, *lendoccupygate.MemStore,) lendoccupygate.PlaceBoundResp
		settleReq	func(lendoccupygate.PlaceBoundResp,) lendoccupygate.SettleReq
		wantErr		bool
		wantErrIs	error
		checkResp	func(*testing.T, *lendoccupygate.SettleResp,)
		checkStore	func(*testing.T, *lendoccupygate.MemStore, lendoccupygate.PlaceBoundResp,)
	}{
		{
			name:	"正常 Settle 关闭租约",
			setup: func(e *lendoccupygate.Engine, _ *lendoccupygate.MemStore,) lendoccupygate.PlaceBoundResp {
				resp, err := e.PlaceBound(context.Background(), lendoccupygate.PlaceBoundReq{
					OrderID:	"ord-settle-001", UserID: "user-s1", Asset: "USDT", Amount: "200",
				})
				if err != nil {
					panic(err)
				}
				return *resp
			},
			settleReq: func(p lendoccupygate.PlaceBoundResp,) lendoccupygate.SettleReq {
				return lendoccupygate.SettleReq{OrderID: p.Lease.OrderID, UserID: p.Lease.UserID, Asset: p.Lease.Asset, Amount: p.Lease.Amount}
			},
			wantErr:	false,
			checkResp: func(t *testing.T, resp *lendoccupygate.SettleResp,) {
				if resp.Lease.State != lendoccupygate.LeaseSettled {
					t.Errorf("lease.State = %s, want settled", resp.Lease.State)
				}
				if resp.DupProof != nil {
					t.Error("dup proof should be nil on success")
				}
			},
			checkStore: func(t *testing.T, store *lendoccupygate.MemStore, p lendoccupygate.PlaceBoundResp,) {
				gen, err := store.GetGen(context.Background(), p.Gen.GenID)
				if err != nil {
					t.Fatalf("GetGen failed: %v", err)
				}
				if !gen.Consumed {
					t.Error("gen should be consumed after settle")
				}
				if gen.ConsumedBy != "settle" {
					t.Errorf("gen.ConsumedBy = %s, want settle", gen.ConsumedBy)
				}
			},
		},
		{
			name:	"金额不匹配返回 ErrAmountMismatch",
			setup: func(e *lendoccupygate.Engine, _ *lendoccupygate.MemStore,) lendoccupygate.PlaceBoundResp {
				resp, _ := e.PlaceBound(context.Background(), lendoccupygate.PlaceBoundReq{
					OrderID:	"ord-settle-amt", UserID: "user-s2", Asset: "USDT", Amount: "100",
				})
				return *resp
			},
			settleReq: func(p lendoccupygate.PlaceBoundResp,) lendoccupygate.SettleReq {
				return lendoccupygate.SettleReq{OrderID: p.Lease.OrderID, UserID: p.Lease.UserID, Asset: p.Lease.Asset, Amount: "999"}
			},
			wantErr:	true,
			wantErrIs:	lendoccupygate.ErrAmountMismatch,
		},
		{
			name:	"用户不匹配返回 ErrOwnerMismatch",
			setup: func(e *lendoccupygate.Engine, _ *lendoccupygate.MemStore,) lendoccupygate.PlaceBoundResp {
				resp, _ := e.PlaceBound(context.Background(), lendoccupygate.PlaceBoundReq{
					OrderID:	"ord-settle-owner", UserID: "user-s3", Asset: "USDT", Amount: "50",
				})
				return *resp
			},
			settleReq: func(p lendoccupygate.PlaceBoundResp,) lendoccupygate.SettleReq {
				return lendoccupygate.SettleReq{OrderID: p.Lease.OrderID, UserID: "wrong-user", Asset: p.Lease.Asset, Amount: p.Lease.Amount}
			},
			wantErr:	true,
			wantErrIs:	lendoccupygate.ErrOwnerMismatch,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubLending{}
			engine, store := newTestEngine(stub)
			placed := tc.setup(engine, store)
			req := tc.settleReq(placed)
			resp, err := engine.Settle(context.Background(), req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Errorf("error = %v, want %v", err, tc.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.checkResp != nil {
				tc.checkResp(t, resp)
			}
			if tc.checkStore != nil {
				tc.checkStore(t, store, placed)
			}
		})
	}
}

// ── 重复 Settle 幂等 ──────────────────────────────────────────────────────

func TestEngine_DuplicateSettle(t *testing.T) {
	stub := &stubLending{}
	engine, store := newTestEngine(stub)
	req := lendoccupygate.PlaceBoundReq{OrderID: "ord-dup-s", UserID: "user-dup", Asset: "USDT", Amount: "100"}
	placed, err := engine.PlaceBound(context.Background(), req)
	if err != nil {
		t.Fatalf("PlaceBound: %v", err)
	}
	settleReq := lendoccupygate.SettleReq{OrderID: req.OrderID, UserID: req.UserID, Asset: req.Asset, Amount: req.Amount}
	resp1, err := engine.Settle(context.Background(), settleReq)
	if err != nil {
		t.Fatalf("first settle: %v", err)
	}
	if resp1.Lease.State != lendoccupygate.LeaseSettled {
		t.Errorf("lease state = %s, want settled", resp1.Lease.State)
	}
	// 第二次 Settle 应落 dup proof
	resp2, err2 := engine.Settle(context.Background(), settleReq)
	if !errors.Is(err2, lendoccupygate.ErrGenConsumed) {
		t.Fatalf("second settle: got error %v, want ErrGenConsumed", err2)
	}
	if resp2.DupProof == nil {
		t.Fatal("second settle: dup proof should not be nil")
	}
	if resp2.DupProof.Kind != lendoccupygate.DupSettleSettle {
		t.Errorf("dup proof kind = %s, want settle_settle", resp2.DupProof.Kind)
	}
	// 账本贷记只发生一次
	if atomic.LoadInt64(&stub.settleCalled) != 1 {
		t.Errorf("settle called %d times, want 1 (no double credit)", stub.settleCalled)
	}
	// 验证 dup proof 已落地
	proof,
		err := store.GetDupProofByGen(context.Background(), placed.Gen.GenID)
	if err != nil {
		t.Fatalf("GetDupProofByGen: %v", err)
	}
	if proof.Kind != lendoccupygate.DupSettleSettle {
		t.Errorf("stored proof kind = %s, want settle_settle", proof.Kind)
	}
}

// ── Cancel ───────────────────────────────────────────────────────────────

func TestEngine_Cancel(t *testing.T) {
	stub := &stubLending{}
	engine, store := newTestEngine(stub)
	req := lendoccupygate.PlaceBoundReq{OrderID: "ord-cancel-001", UserID: "user-c1", Asset: "ETH", Amount: "2.5"}
	placed, err := engine.PlaceBound(context.Background(), req)
	if err != nil {
		t.Fatalf("PlaceBound: %v", err)
	}
	cancelReq := lendoccupygate.CancelReq{OrderID: req.OrderID,
		UserID:	req.UserID, Asset: req.Asset, Amount: req.Amount}
	resp, err := engine.Cancel(context.Background(), cancelReq)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if resp.Lease.State != lendoccupygate.LeaseCancelled {
		t.Errorf("lease.State = %s, want cancelled", resp.Lease.State)
	}
	gen, _ := store.GetGen(context.Background(), placed.Gen.GenID)
	if !gen.Consumed || gen.ConsumedBy != "cancel" {
		t.Errorf("gen consumed=%v, consumedBy=%s, want consumed=true, cancel", gen.Consumed, gen.ConsumedBy)
	}
}

// ── 并发 Settle+Cancel 竞态 ───────────────────────────────────────────────

func TestEngine_ConcurrentSettleCancel(t *testing.T) {
	const iterations = 5
	for i := 0; i < iterations; i++ {
		i := i
		t.Run(fmt.Sprintf("iter_%d", i), func(t *testing.T) {
			t.Parallel()
			runConcurrentSettleCancel(t, fmt.Sprintf("ord-race-%d", i))
		})
	}
}

func runConcurrentSettleCancel(t *testing.T, orderID string) {
	t.Helper()
	stub := &stubLending{}
	engine, store := newTestEngine(stub)
	req := lendoccupygate.PlaceBoundReq{OrderID: orderID, UserID: "user-race", Asset: "USDT", Amount: "100"}
	placed, err := engine.PlaceBound(context.Background(), req)
	if err != nil {
		t.Fatalf("PlaceBound: %v", err)
	}
	settleReq := lendoccupygate.SettleReq{OrderID: orderID, UserID: req.UserID, Asset: req.Asset, Amount: req.Amount}
	cancelReq := lendoccupygate.CancelReq{OrderID: orderID, UserID: req.UserID, Asset: req.Asset, Amount: req.Amount}
	var (
		settleOK	int64
		cancelOK	int64
		settleResp	*lendoccupygate.SettleResp
		cancelResp	*lendoccupygate.CancelResp
		mu		sync.Mutex
		wg		sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		resp, err := engine.Settle(context.Background(), settleReq)
		if err == nil {
			atomic.AddInt64(&settleOK, 1)
			mu.Lock()
			settleResp = resp
			mu.Unlock()
		} else if !errors.Is(err, lendoccupygate.ErrGenConsumed) {
			t.Errorf("settle unexpected error: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		resp, err := engine.Cancel(context.Background(), cancelReq)
		if err == nil {
			atomic.AddInt64(&cancelOK, 1)
			mu.Lock()
			cancelResp = resp
			mu.Unlock()
		} else if !errors.Is(err, lendoccupygate.ErrGenConsumed) {
			t.Errorf("cancel unexpected error: %v", err)
		}
	}()
	wg.Wait()
	// 只能有一方成功
	total := atomic.LoadInt64(&settleOK) + atomic.LoadInt64(&cancelOK)
	if total != 1 {
		t.Errorf("exactly 1 winner expected, got settle=%d cancel=%d", settleOK, cancelOK)
	}
	// 账本操作只发生一次
	creditOps := atomic.LoadInt64(&stub.settleCalled) + atomic.LoadInt64(&stub.cancelCalled)
	if creditOps != 1 {
		t.Errorf("credit ops = %d, want 1 (no double credit)", creditOps)
	}
	// gen 已消费
	gen, _ := store.GetGen(context.Background(), placed.Gen.GenID)
	if !gen.Consumed {
		t.Error("gen should be consumed after race")
	}
	// 失败方有 dup proof
	mu.Lock()
	hasSettleResp := settleResp != nil
	hasCancelResp := cancelResp != nil
	mu.Unlock()
	// 验证落地证明
	if atomic.LoadInt64(&settleOK) == 0 {
		// settle 失败，cancel 成功：应有 settle_cancel proof
		proof, err := store.GetDupProofByGen(context.Background(), placed.Gen.GenID)
		if err != nil {
			t.Fatalf("expected dup proof after race, got error: %v", err)
		}
		if proof.Kind != lendoccupygate.DupSettleCancel {
			t.Errorf("proof kind = %s, want settle_cancel", proof.Kind)
		}
	}
	if atomic.LoadInt64(&cancelOK) == 0 {
		// cancel 失败，settle 成功：应有 settle_cancel proof
		proof, err := store.GetDupProofByGen(context.Background(), placed.Gen.GenID)
		if err != nil {
			t.Fatalf("expected dup proof after race, got error: %v", err)
		}
		if proof.Kind != lendoccupygate.DupSettleCancel {
			t.Errorf("proof kind = %s, want settle_cancel", proof.Kind)
		}
	}
	_ = hasSettleResp
	_ = hasCancelResp
}

// ── 半截补偿 ──────────────────────────────────────────────────────────────

func TestEngine_CompensateResidue(t *testing.T) {
	t.Run("正常半截补偿：placed 租约被补偿关闭", func(t *testing.T) {
		stub := &stubLending{}
		engine, store := newTestEngine(stub)
		// 先正常 PlaceBound 建立租约
		req := lendoccupygate.PlaceBoundReq{OrderID: "ord-residue-001", UserID: "user-r1", Asset: "USDT", Amount: "80"}
		placed, err := engine.PlaceBound(context.Background(), req)
		if err != nil {
			t.Fatalf("PlaceBound: %v", err)
		}
		// 直接对 placed 租约触发补偿
		lease, _ := store.GetLeaseByOrder(context.Background(), req.OrderID)
		proof,
			err := engine.CompensateResidue(context.Background(), lease)
		if err != nil {
			t.Fatalf("CompensateResidue: %v", err)
		}
		if proof.GenID != placed.Gen.GenID {
			t.Errorf("proof.GenID = %s, want %s", proof.GenID, placed.Gen.GenID)
		}
		// 租约应已关闭
		updatedLease, _ := store.GetLeaseByOrder(context.Background(), req.OrderID)
		if updatedLease.State != lendoccupygate.LeaseResidueClosed {
			t.Errorf("lease.State = %s, want residue_closed", updatedLease.State)
		}
		// 幂等：再次补偿返回相同证明
		proof2, err := engine.CompensateResidue(context.Background(), lease)
		if err != nil {
			t.Fatalf("second CompensateResidue: %v", err)
		}
		if proof2.ProofID != proof.ProofID {
			t.Error("idempotent: proof ID should be same on second call")
		}
		// 补偿操作不得再次执行
		if atomic.LoadInt64(&stub.compensateCalled) != 1 {
			t.Errorf("compensate called %d times, want 1 (idempotent)", stub.compensateCalled)
		}
	})
	t.Run("非 placed 状态租约返回 ErrLeaseNotPlaced", func(t *testing.T) {
		stub := &stubLending{}
		engine, store := newTestEngine(stub)
		req := lendoccupygate.PlaceBoundReq{OrderID: "ord-residue-002", UserID: "user-r2", Asset: "BTC", Amount: "0.1"}
		placed, err := engine.PlaceBound(context.Background(), req)
		if err != nil {
			t.Fatalf("PlaceBound: %v", err)
		}
		_ = placed
		// 先 Settle 关闭
		sReq := lendoccupygate.SettleReq{OrderID: req.OrderID, UserID: req.UserID, Asset: req.Asset, Amount: req.Amount}
		_, err = engine.Settle(context.Background(), sReq)
		if err != nil {
			t.Fatalf("Settle: %v", err)
		}
		// 对 settled 租约补偿应失败
		lease, _ := store.GetLeaseByOrder(context.Background(), req.OrderID)
		_,
			err = engine.CompensateResidue(context.Background(), lease)
		if !errors.Is(err, lendoccupygate.ErrLeaseNotPlaced) {
			t.Errorf("expected ErrLeaseNotPlaced, got %v", err)
		}
	})
}

// ── ExportProofs ─────────────────────────────────────────────────────────

func TestEngine_ExportProofs(t *testing.T) {
	t.Run("空 OrderID 返回错误", func(t *testing.T) {
		stub := &stubLending{}
		engine, _ := newTestEngine(stub)
		_, err := engine.ExportProofs(context.Background(), lendoccupygate.ExportProofReq{OrderID: ""})
		if !errors.Is(err, lendoccupygate.ErrOrderRequired) {
			t.Errorf("expected ErrOrderRequired, got %v", err)
		}
	})
	t.Run("无证明时返回空列表", func(t *testing.T) {
		stub := &stubLending{}
		engine, _ := newTestEngine(stub)
		resp, err := engine.ExportProofs(context.Background(), lendoccupygate.ExportProofReq{OrderID: "ord-no-proof"})
		if err != nil {
			t.Fatalf("ExportProofs: %v", err)
		}
		if len(resp.DupProofs) != 0 || len(resp.ResidueProofs) != 0 {
			t.Error("expected empty proof lists")
		}
	})
	t.Run("重复 Settle 后导出 dup proof", func(t *testing.T) {
		stub := &stubLending{}
		engine, _ := newTestEngine(stub)
		req := lendoccupygate.PlaceBoundReq{OrderID: "ord-exp-001", UserID: "user-exp", Asset: "USDT", Amount: "10"}
		_, err := engine.PlaceBound(context.Background(), req)
		if err != nil {
			t.Fatalf("PlaceBound: %v", err)
		}
		sReq := lendoccupygate.SettleReq{OrderID: req.OrderID, UserID: req.UserID, Asset: req.Asset, Amount: req.Amount}
		_, _ = engine.Settle(context.Background(), sReq)
		engine.Settle(context.Background(),	//nolint:errcheck
			sReq)
		resp, err := engine.ExportProofs(context.Background(), lendoccupygate.ExportProofReq{OrderID: req.OrderID})
		if err != nil {
			t.Fatalf("ExportProofs: %v", err)
		}
		if len(resp.DupProofs) != 1 {
			t.Errorf("dup proofs count = %d, want 1", len(resp.DupProofs))
		}
	})
}

// ── MemStore 状态机 ───────────────────────────────────────────────────────

func TestMemStore_ConsumeGen_CAS(t *testing.T) {
	store := lendoccupygate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	gen := &lendoccupygate.LendGen{
		GenID:		"gen-cas-001",
		OrderID:	"ord-cas-001",
		UserID:		"user-cas",
		Asset:		"USDT",
		Amount:		"50",
		IssuedAt:	now,
		CreatedAt:	now,
	}
	if err := store.SaveGen(ctx, gen); err != nil {
		t.Fatalf("SaveGen: %v", err)
	}
	// 第一次消费成功
	if err := store.ConsumeGen(ctx, gen.GenID, "settle", now); err != nil {
		t.Fatalf("first ConsumeGen: %v", err)
	}
	// 第二次消费失败
	if err := store.ConsumeGen(ctx, gen.GenID, "cancel", now); !errors.Is(err, lendoccupygate.ErrGenConsumed) {
		t.Errorf("second ConsumeGen: got %v, want ErrGenConsumed", err)
	}
	// 并发多个 goroutine 只有一个成功
	store2 := lendoccupygate.NewMemStore()
	gen2 := &lendoccupygate.LendGen{GenID: "gen-cas-002", OrderID: "ord-cas-002", UserID: "u", Asset: "BTC", Amount: "1", IssuedAt: now, CreatedAt: now}
	store2.SaveGen(ctx, gen2)	//nolint:errcheck
	var successCount int64
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store2.ConsumeGen(ctx, gen2.GenID, "settle", now); err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}
	wg.Wait()
	if atomic.LoadInt64(&successCount) != 1 {
		t.Errorf("concurrent ConsumeGen: %d succeeded, want exactly 1", successCount)
	}
}

func TestMemStore_UpdateLeaseState_CAS(t *testing.T) {
	store := lendoccupygate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	lease := &lendoccupygate.LendOccupyLease{
		LeaseID:	"lease-cas-001",
		GenID:		"gen-x",
		OrderID:	"ord-cas-x",
		UserID:		"user-x",
		Asset:		"USDT",
		Amount:		"10",
		State:		lendoccupygate.LeasePlaced,
		PlacedAt:	now,
		CreatedAt:	now,
		UpdatedAt:	now,
	}
	store.SaveLease(ctx, lease)	//nolint:errcheck
	// 正确迁移：placed → settled
	t1 := now.Add(time.Second)
	if err := store.UpdateLeaseState(ctx, lease.LeaseID, lendoccupygate.LeasePlaced, lendoccupygate.LeaseSettled, &lendoccupygate.LendOccupyLease{SettledAt: &t1}); err != nil {
		t.Fatalf("UpdateLeaseState: %v", err)
	}
	// 再次从 placed → cancelled 应失败
	if err := store.UpdateLeaseState(ctx, lease.LeaseID, lendoccupygate.LeasePlaced, lendoccupygate.LeaseCancelled, nil); !errors.Is(err, lendoccupygate.ErrInvalidTransition) {
		t.Errorf("second UpdateLeaseState: got %v, want ErrInvalidTransition", err)
	}
}

func TestMemStore_SaveDupProof_Idempotent(t *testing.T) {
	store := lendoccupygate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	proof := &lendoccupygate.LendDupReleaseProof{
		ProofID:		"proof-dup-001",
		GenID:			"gen-dup-001",
		OrderID:		"ord-dup-001",
		UserID:			"user-dup",
		Asset:			"USDT",
		Amount:			"100",
		Kind:			lendoccupygate.DupSettleSettle,
		FirstConsumedBy:	"settle",
		FirstConsumedAt:	now,
		AttemptedBy:		"settle",
		AttemptedAt:		now,
		CreatedAt:		now,
	}
	if err := store.SaveDupProof(ctx, proof); err != nil {
		t.Fatalf("first SaveDupProof: %v", err)
	}
	if err := store.SaveDupProof(ctx, proof); !errors.Is(err, lendoccupygate.ErrAlreadyProven) {
		t.Errorf("second SaveDupProof: got %v, want ErrAlreadyProven", err)
	}
}
