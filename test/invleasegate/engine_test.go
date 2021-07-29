package invleasegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/invleasegate"
)

// ─── stub DriftChecker ────────────────────────────────────────────────────────

type stubChecker struct {
	hasActive bool
	err error
}

  func (s *stubChecker) HasActiveAdvice(_ context.Context, _ string) (bool, error) {
	   return s.hasActive, s.err
  }

// ─── 工具 ──────────────────────────────────────────────────────────────────────

  func newEngine(t *testing.T, checker invleasegate.DriftChecker, opts ...invleasegate.EngineOption) (*invleasegate.Engine, *invleasegate.MemStore) {
	  t.Helper()
	  store := invleasegate.NewMemStore()
	   e := invleasegate.NewEngine(store, checker, opts...)
	  return e, store
   }

  func fixedClock(base time.Time) func() time.Time {
	  return func() time.Time { return base }
  }

// ─── PrecheckBind 表驱动 ──────────────────────────────────────────────────

   func TestPrecheckBind(t *testing.T) {
	  base := time.Date(2026,
		  3, 15, 10, 0, 0, 0, time.UTC)
	  cases := []struct {
		   name    string
		   checker invleasegate.DriftChecker
		  req       invleasegate.PrecheckBindReq
		  wantState invleasegate.LeaseState
		   wantErr error
		   wantHasActive bool
	   }{
		  {
			   name:    "正常路径_无活跃建议",
			   checker: &stubChecker{hasActive: false},
			  req:       invleasegate.PrecheckBindReq{Asset: "BTC", Actor: "trader-001"},
			  wantState: invleasegate.LeaseIssued,
			  wantErr:   nil,
			   wantHasActive: false,
		   },
		   {
			   name:    "活跃建议_拒绝签发",
			   checker: &stubChecker{hasActive: true},
			  req:     invleasegate.PrecheckBindReq{Asset: "USDT", Actor: "trader-002"},
			  wantErr: invleasegate.ErrActiveAdvice,
		  },
		  {
			  name:    "checker不可用_fail-closed",
			  checker: &stubChecker{err: errors.New("connection refused")},
			   req:     invleasegate.PrecheckBindReq{Asset: "ETH", Actor: "trader-003"},
			  wantErr: invleasegate.ErrCheckerDown,
		  },
		   {
			  name:    "checker为nil_fail-closed",
			  checker: nil,
			  req:     invleasegate.PrecheckBindReq{Asset: "BTC", Actor: "trader-004"},
			   wantErr: invleasegate.ErrCheckerDown,
		   },
		   {
			  name: "asset为空_拒绝",
			   checker: &stubChecker{hasActive: false},
			  req:     invleasegate.PrecheckBindReq{Asset: "", Actor: "trader-005"},
			   wantErr: invleasegate.ErrAsset,
		   },
		   {
			  name:    "actor为空_拒绝",
			   checker: &stubChecker{hasActive: false},
			  req:     invleasegate.PrecheckBindReq{Asset: "BTC", Actor: ""},
			  wantErr: invleasegate.ErrActor,
		  },
		   {
			   name: "asset小写自动规范化",
			   checker: &stubChecker{hasActive: false},
			  req:       invleasegate.PrecheckBindReq{Asset: "btc", Actor: "trader-006"},
			  wantState: invleasegate.LeaseIssued,
			   wantErr: nil,
		  },
		  {
			  name: "自定义TTL在合法范围内",
			  checker:   &stubChecker{hasActive: false},
			  req:       invleasegate.PrecheckBindReq{Asset: "ETH", Actor: "trader-007", LeaseTTL: 60 * time.Second},
			   wantState: invleasegate.LeaseIssued,
			   wantErr:   nil,
		   },
	  }
	   for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			   e, store := newEngine(t, tc.checker, invleasegate.WithClock(fixedClock(base)))
			  ctx := context.Background()
			   res, err := e.PrecheckBind(ctx, tc.req)
			  if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					  t.Fatalf("PrecheckBind error = %v, wantErr %v", err, tc.wantErr)
				  }
				   if res.Lease != nil {
					  t.Fatal("PrecheckBind 错误路径不应返回租约")
				   }
				   return
			   }
			   if err != nil {
				   t.Fatalf("PrecheckBind unexpected error: %v", err)
			   }
			  if res.Lease == nil {
				   t.Fatal("PrecheckBind 应返回租约")
			   }
			  if res.Lease.State != tc.wantState {
				   t.Errorf("state = %v, want %v", res.Lease.State, tc.wantState)
			   }
			  if res.Lease.LeaseID == "" {
				   t.Error("LeaseID 不应为空")
			  }
			   if res.Lease.FingerprintAtIssue.HasActive != tc.wantHasActive {
				  t.Errorf("fingerprint.HasActive = %v, want %v",
					   res.Lease.FingerprintAtIssue.HasActive, tc.wantHasActive)
			   }
			   // 验证持久化
			  saved, err := store.GetLease(ctx, res.Lease.LeaseID)
			  if err != nil {
				  t.Fatalf("GetLease error: %v", err)
			  }
			   if saved.State != tc.wantState {
				  t.Errorf("saved state = %v, want %v", saved.State, tc.wantState)
			  }
		   })
	  }
   }

// ─── CommitRecheck 表驱动 ─────────────────────────────────────────────────

  func TestCommitRecheck(t *testing.T) {
	   base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	  cases := []struct {
		  name string
		  checkerAtBind   *stubChecker  // bind 时 checker 状态
		   checkerAtCommit *stubChecker  // commit 时 checker 状态
		  advanceTime     time.Duration // bind 后推进时钟
		  commitReq func(leaseID string) invleasegate.CommitRecheckReq
		  wantState invleasegate.LeaseState
		  wantErr   error
		   wantProofKind invleasegate.DriftKind
	  }{
		   {
			  name:            "正常路径_commit成功",
			   checkerAtBind:   &stubChecker{hasActive: false},
			   checkerAtCommit: &stubChecker{hasActive: false},
			  advanceTime: 0,
			   commitReq: func(id string) invleasegate.CommitRecheckReq {
				  return invleasegate.CommitRecheckReq{LeaseID: id, Asset: "BTC", Actor: "trader-001"}
			  },
			   wantState: invleasegate.LeaseCommitted,
			   wantErr:   nil,
		   },
		   {
			  name:          "无lease_id_拒绝",
			   checkerAtBind: &stubChecker{hasActive: false},
			   commitReq:     func(_ string) invleasegate.CommitRecheckReq { return invleasegate.CommitRecheckReq{LeaseID: ""} },
			  wantErr:       invleasegate.ErrNoLease,
		   },
		  {
			   name:            "lease过期_拒绝",
			  checkerAtBind:   &stubChecker{hasActive: false},
			   checkerAtCommit: &stubChecker{hasActive: false},
			   advanceTime:     400 * time.Second, // 超过 defaultLeaseTTL(300s)
			   commitReq: func(id string) invleasegate.CommitRecheckReq {
				   return invleasegate.CommitRecheckReq{LeaseID: id, Asset: "BTC"}
			  },
			  wantState: invleasegate.LeaseBlocked,
			   wantErr:       invleasegate.ErrLeaseExpired,
			   wantProofKind: invleasegate.DriftCommitNoLease,
		  },
		   {
			   name: "建议翻转_commit拒绝_落proof",
			  checkerAtBind: &stubChecker{hasActive: false},
			  checkerAtCommit: &stubChecker{hasActive: true}, // 翻转：无→有
			   advanceTime: 0,
			   commitReq: func(id string) invleasegate.CommitRecheckReq {
				   return invleasegate.CommitRecheckReq{LeaseID: id, Asset: "BTC", CommitRef: "wd-99001"}
			  },
			   wantState: invleasegate.LeaseBlocked,
			   wantErr:   invleasegate.ErrFingerprintMismatch,
			   wantProofKind: invleasegate.DriftCommitFingerMismatch,
		  },
		  {
			  name:            "checker故障_fail-closed_落proof",
			   checkerAtBind:   &stubChecker{hasActive: false},
			  checkerAtCommit: &stubChecker{err: errors.New("timeout")},
			  advanceTime:     0,
			   commitReq: func(id string) invleasegate.CommitRecheckReq {
				  return invleasegate.CommitRecheckReq{LeaseID: id, Asset: "BTC"}
			  },
			  wantState: invleasegate.LeaseBlocked,
			   wantErr:   invleasegate.ErrCheckerDown,
			  wantProofKind: invleasegate.DriftCheckerDown,
		  },
		  {
			  name:            "不存在的lease_id",
			  checkerAtBind:   &stubChecker{hasActive: false},
			   checkerAtCommit: &stubChecker{hasActive: false},
			   commitReq: func(_ string) invleasegate.CommitRecheckReq {
				   return invleasegate.CommitRecheckReq{LeaseID: "nonexistent-id"}
			   },
			  wantErr: invleasegate.ErrNoLease,
		   },
		  {
			  name: "重复commit_idempotent拒绝",
			   checkerAtBind:   &stubChecker{hasActive: false},
			   checkerAtCommit: &stubChecker{hasActive: false},
			   advanceTime: 0,
			   commitReq: func(id string) invleasegate.CommitRecheckReq {
				  return invleasegate.CommitRecheckReq{LeaseID: id, Asset: "BTC"}
			  },
			  wantState: invleasegate.LeaseCommitted,
			   wantErr: nil,
			  // 第二次 commit 会在下面额外
		  },
	  }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   ctx := context.Background()
			  var now = base
			   clk := func() time.Time { return now }
			   store := invleasegate.NewMemStore().WithClock(clk)
			  // bind 阶段
			  bindEngine := invleasegate.NewEngine(store, tc.checkerAtBind, invleasegate.WithClock(clk))
			  bindRes, bindErr := bindEngine.PrecheckBind(ctx, invleasegate.PrecheckBindReq{
				  Asset: "BTC", Actor: "tester",
			   })
			   var leaseID string
			   if bindErr == nil && bindRes.Lease != nil {
				   leaseID = bindRes.Lease.LeaseID
			  }
			   // 推进时钟
			   if tc.advanceTime > 0 {
				  now = now.Add(tc.advanceTime)
			   }
			  // commit 阶段（用不同 checker）
			  commitEngine := invleasegate.NewEngine(store, tc.checkerAtCommit, invleasegate.WithClock(clk))
			  req := tc.commitReq(leaseID)
			  res, err := commitEngine.CommitRecheck(ctx, req)
			   if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					  t.Fatalf("CommitRecheck error = %v, wantErr %v", err, tc.wantErr)
				   }
				  // 验证租约终态
				  if leaseID != "" && tc.wantState != "" {
					  saved, getErr := store.GetLease(ctx, leaseID)
					  if getErr == nil && saved.State != tc.wantState {
						  t.Errorf("lease state = %v, want %v", saved.State, tc.wantState)
					  }
				   }
				   // 验证漂移证明
				   if tc.wantProofKind != "" && leaseID != "" {
					   proof, pErr := store.GetProofByLease(ctx, leaseID)
					   if pErr != nil {
						   t.Fatalf("GetProofByLease error: %v", pErr)
					   }
					   if proof.Kind != tc.wantProofKind {
						   t.Errorf("proof.Kind = %v, want %v", proof.Kind, tc.wantProofKind)
					   }
					   if proof.State != invleasegate.ProofOpen {
						  t.Errorf("proof.State = %v, want open", proof.State)
					   }
				   }
				  return
			   }
			  if err != nil {
				   t.Fatalf("CommitRecheck unexpected error: %v", err)
			  }
			   if res.Lease == nil {
				  t.Fatal("CommitRecheck 应返回租约")
			  }
			   if res.Lease.State != tc.wantState {
				   t.Errorf("state = %v, want %v", res.Lease.State, tc.wantState)
			   }
		   })
	  }
   }

// ─── 重复 Commit 幂等拒绝 ─────────────────────────────────────────────────

  func TestCommitRecheck_DoubleCommit(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	  base := time.Date(2026,
		   3, 15, 10, 0, 0, 0, time.UTC)
	   checker := &stubChecker{hasActive: false}
	   e, _ := newEngine(t, checker, invleasegate.WithClock(fixedClock(base)))
	   bindRes, err := e.PrecheckBind(ctx, invleasegate.PrecheckBindReq{Asset: "BTC", Actor: "tester"})
	   if err != nil {
		  t.Fatalf("PrecheckBind error: %v", err)
	   }
	  leaseID := bindRes.Lease.LeaseID
	  req := invleasegate.CommitRecheckReq{LeaseID: leaseID, Asset: "BTC", Actor: "tester"}
	   _, err = e.CommitRecheck(ctx, req)
	   if err != nil {
		  t.Fatalf("第一次 CommitRecheck error: %v", err)
	  }
	  // 第二次 commit 必须拒绝（终态）
	   _, err = e.CommitRecheck(ctx, req)
	  if !errors.Is(err, invleasegate.ErrLeaseTerminal) {
		  t.Errorf("第二次 CommitRecheck error = %v, wantErr ErrLeaseTerminal", err)
	   }
  }

// ─── AdviceFlipGuard 表驱动 ───────────────────────────────────────────────

  func TestAdviceFlipGuard(t *testing.T) {
	  base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	   cases := []struct {
		   name           string
		  checkerAtBind  *stubChecker
		   checkerAtGuard *stubChecker
		  advanceTime time.Duration
		   wantLeaseState invleasegate.LeaseState
		  wantProofKind  invleasegate.DriftKind
		  wantErr        error
	  }{
		   {
			  name:           "无翻转_租约保持issued",
			   checkerAtBind:  &stubChecker{hasActive: false},
			  checkerAtGuard: &stubChecker{hasActive: false},
			   wantLeaseState: invleasegate.LeaseIssued,
		  },
		  {
			  name:           "建议翻转无→有_封堵",
			  checkerAtBind:  &stubChecker{hasActive: false},
			  checkerAtGuard: &stubChecker{hasActive: true},
			   wantLeaseState: invleasegate.LeaseBlocked,
			  wantProofKind: invleasegate.DriftAdviceFlip,
		   },
		  {
			   name:           "guard时checker故障_fail-closed封堵",
			   checkerAtBind:  &stubChecker{hasActive: false},
			   checkerAtGuard: &stubChecker{err: errors.New("timeout")},
			  wantLeaseState: invleasegate.LeaseBlocked,
			  wantProofKind: invleasegate.DriftCheckerDown,
		   },
		   {
			   name:          "租约已过期_guard推进expired",
			   checkerAtBind: &stubChecker{hasActive: false},
			   checkerAtGuard: &stubChecker{hasActive: false},
			   advanceTime:    400 * time.Second,
			   wantLeaseState: invleasegate.LeaseExpired,
		  },
		  {
			   name: "guard已终态_幂等返回nil",
			   checkerAtBind: &stubChecker{hasActive: false},
			  checkerAtGuard: &stubChecker{hasActive: true}, // 第一次翻转封堵
			   wantLeaseState: invleasegate.LeaseBlocked,
			   wantProofKind:  invleasegate.DriftAdviceFlip,
		  },
	  }
	   for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  ctx := context.Background()
			   var now = base
			  clk := func() time.Time { return now }
			  store := invleasegate.NewMemStore().WithClock(clk)
			  bindEngine := invleasegate.NewEngine(store, tc.checkerAtBind, invleasegate.WithClock(clk))
			   bindRes, err := bindEngine.PrecheckBind(ctx, invleasegate.PrecheckBindReq{Asset: "BTC", Actor: "tester"})
			  if err != nil {
				   t.Fatalf("PrecheckBind error: %v", err)
			   }
			  leaseID := bindRes.Lease.LeaseID
			  if tc.advanceTime > 0 {
				  now = now.Add(tc.advanceTime)
			  }
			  guardEngine := invleasegate.NewEngine(store, tc.checkerAtGuard, invleasegate.WithClock(clk))
			  guardErr := guardEngine.AdviceFlipGuard(ctx, invleasegate.AdviceFlipGuardReq{
				  LeaseID: leaseID,
				  Actor:   "invleasegate.worker",
			  })
			  if tc.wantErr != nil {
				   if !errors.Is(guardErr, tc.wantErr) {
					   t.Fatalf("AdviceFlipGuard error = %v, wantErr %v", guardErr, tc.wantErr)
				  }
			  } else if guardErr != nil {
				  t.Fatalf("AdviceFlipGuard unexpected error: %v", guardErr)
			  }
			   saved, err := store.GetLease(ctx, leaseID)
			  if err != nil {
				   t.Fatalf("GetLease error: %v", err)
			  }
			  if saved.State != tc.wantLeaseState {
				   t.Errorf("state = %v, want %v", saved.State, tc.wantLeaseState)
			   }
			  if tc.wantProofKind != "" {
				  proof, pErr := store.GetProofByLease(ctx, leaseID)
				  if pErr != nil {
					   t.Fatalf("GetProofByLease error: %v", pErr)
				  }
				   if proof.Kind != tc.wantProofKind {
					  t.Errorf("proof.Kind = %v, want %v", proof.Kind, tc.wantProofKind)
				  }
			  }
		   })
	  }
   }

// ─── 幂等封堵（并发竞态防护） ────────────────────────────────────────────

  func TestBlockLease_Idempotent(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	  base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	  checker := &stubChecker{hasActive: false}
	   e, store := newEngine(t, checker, invleasegate.WithClock(fixedClock(base)))
	  bindRes, err := e.PrecheckBind(ctx, invleasegate.PrecheckBindReq{Asset: "BTC", Actor: "tester"})
	  if err != nil {
		  t.Fatalf("PrecheckBind error: %v", err)
	   }
	  leaseID := bindRes.Lease.LeaseID
	   // 第一次翻转封堵
	   flipChecker := &stubChecker{hasActive: true}
	   flipEngine := invleasegate.NewEngine(store, flipChecker, invleasegate.WithClock(fixedClock(base)))
	  if err := flipEngine.AdviceFlipGuard(ctx, invleasegate.AdviceFlipGuardReq{LeaseID: leaseID, Actor: "worker"}); err != nil {
		  t.Fatalf("第一次 AdviceFlipGuard error: %v", err)
	   }
	  saved, _ := store.GetLease(ctx, leaseID)
	  if saved.State != invleasegate.LeaseBlocked {
		   t.Fatalf("封堵后 state = %v, want blocked", saved.State)
	  }
	   // 第二次封堵（幂等，不应返回错误，不应重复落 proof）
	  if err := flipEngine.AdviceFlipGuard(ctx, invleasegate.AdviceFlipGuardReq{LeaseID: leaseID, Actor: "worker"}); err != nil {
		   t.Fatalf("第二次 AdviceFlipGuard error: %v", err)
	  }
	   // proof 应仍为 1 条
	   proofs, err := store.ListOpenProofs(ctx, 10)
	  if err != nil {
		   t.Fatalf("ListOpenProofs error: %v", err)
	   }
	   if len(proofs) != 1 {
		  t.Errorf("open proofs = %d, want 1（幂等）", len(proofs))
	  }
   }

// ─── CloseProof ──────────────────────────────────────────────────────────

   func TestCloseProof(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	  base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	   checker := &stubChecker{hasActive: false}
	   e, store := newEngine(t, checker, invleasegate.WithClock(fixedClock(base)))
	   bindRes, _ := e.PrecheckBind(ctx, invleasegate.PrecheckBindReq{Asset: "BTC", Actor: "tester"})
	  leaseID := bindRes.Lease.LeaseID
	  flipEngine := invleasegate.NewEngine(store, &stubChecker{hasActive: true}, invleasegate.WithClock(fixedClock(base)))
	  _ = flipEngine.AdviceFlipGuard(ctx, invleasegate.AdviceFlipGuardReq{LeaseID: leaseID, Actor: "worker"})
	  proof,
		   err := store.GetProofByLease(ctx, leaseID)
	  if err != nil {
		  t.Fatalf("GetProofByLease error: %v", err)
	  }
	  if proof.State != invleasegate.ProofOpen {
		   t.Fatalf("初始 proof state = %v, want open", proof.State)
	   }
	  closed, err := e.CloseProof(ctx, proof.ProofID, "auditor-001")
	   if err != nil {
		   t.Fatalf("CloseProof error: %v", err)
	   }
	   if closed.State != invleasegate.ProofClosed {
		   t.Errorf("closed proof state = %v, want closed", closed.State)
	  }
	   if closed.ClosedAt == nil {
		   t.Error("closed_at 不应为空")
	   }
	  // 再次关闭（幂等）
	   closed2, err := e.CloseProof(ctx, proof.ProofID, "auditor-001")
	   if err != nil {
		   t.Fatalf("CloseProof 幂等 error: %v", err)
	  }
	  if closed2.State != invleasegate.ProofClosed {
		  t.Errorf("幂等关闭后 state = %v, want closed", closed2.State)
	  }
  }

// ─── Fail-closed 端到端语义 ───────────────────────────────────────────────

   func TestFailClosed_EndToEnd(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	  base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	   // checker 在预检时正常，commit 时故障
	   upChecker := &stubChecker{hasActive: false}
	  downChecker := &stubChecker{err: errors.New("service unavailable")}
	   store := invleasegate.NewMemStore().WithClock(fixedClock(base))
	   bindEngine := invleasegate.NewEngine(store, upChecker, invleasegate.WithClock(fixedClock(base)))
	   bindRes, err := bindEngine.PrecheckBind(ctx, invleasegate.PrecheckBindReq{Asset: "USDT", Actor: "trader-006"})
	   if err != nil {
		  t.Fatalf("PrecheckBind error: %v", err)
	  }
	   leaseID := bindRes.Lease.LeaseID
	   // checker 故障时提交
	   commitEngine := invleasegate.NewEngine(store,
		  downChecker, invleasegate.WithClock(fixedClock(base)))
	  _, err = commitEngine.CommitRecheck(ctx, invleasegate.CommitRecheckReq{
		   LeaseID: leaseID,
		   Asset:   "USDT",
		  Actor:     "trader-006",
		  CommitRef: "wd-txn-99002",
	   })
	   if !errors.Is(err, invleasegate.ErrCheckerDown) {
		   t.Errorf("fail-closed commit error = %v, wantErr ErrCheckerDown", err)
	  }
	   // 验证租约 blocked
	   saved, _ := store.GetLease(ctx, leaseID)
	  if saved.State != invleasegate.LeaseBlocked {
		   t.Errorf("fail-closed 后 state = %v, want blocked", saved.State)
	  }
	   if saved.BlockReason != string(invleasegate.DriftCheckerDown) {
		  t.Errorf("block_reason = %v, want checker_down", saved.BlockReason)
	   }
	  // 验证漂移证明
	   proof, err := store.GetProofByLease(ctx, leaseID)
	   if err != nil {
		   t.Fatalf("GetProofByLease error: %v", err)
	  }
	   if proof.Kind != invleasegate.DriftCheckerDown {
		  t.Errorf("proof.Kind = %v, want checker_down", proof.Kind)
	   }
	   if proof.CommitRef != "wd-txn-99002" {
		  t.Errorf("proof.CommitRef = %v, want wd-txn-99002", proof.CommitRef)
	   }
	  // 验证已 blocked 的租约不能再 commit
	  _, err = commitEngine.CommitRecheck(ctx, invleasegate.CommitRecheckReq{LeaseID: leaseID, Asset: "USDT"})
	  if !errors.Is(err, invleasegate.ErrLeaseBlocked) {
		  t.Errorf("再次 commit 已封堵租约 error = %v, wantErr ErrLeaseBlocked", err)
	   }
  }

// ─── 资产交叉校验 ─────────────────────────────────────────────────────────

   func TestCommitRecheck_AssetMismatch(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	  base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	   checker := &stubChecker{hasActive: false}
	   e, _ := newEngine(t, checker, invleasegate.WithClock(fixedClock(base)))
	  bindRes, _ := e.PrecheckBind(ctx, invleasegate.PrecheckBindReq{Asset: "BTC", Actor: "tester"})
	   leaseID := bindRes.Lease.LeaseID
	   _, err := e.CommitRecheck(ctx, invleasegate.CommitRecheckReq{
		  LeaseID: leaseID,
		  Asset:   "ETH", // 资产不匹配
	   })
	   if !errors.Is(err, invleasegate.ErrAsset) {
		  t.Errorf("资产不匹配 error = %v, wantErr ErrAsset", err)
	  }
   }

// ─── MemStore 幂等性 ──────────────────────────────────────────────────────

  func TestMemStore_SaveProof_Idempotent(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	  store := invleasegate.NewMemStore()
	   now := time.Now().UTC()
	  proof := &invleasegate.InvGateDriftProof{
		  ProofID: "proof-001",
		  LeaseID: "lease-001",
		   Asset: "BTC",
		   Kind: invleasegate.DriftAdviceFlip,
		  State: invleasegate.ProofOpen,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if err := store.SaveProof(ctx, proof); err != nil {
		  t.Fatalf("第一次 SaveProof error: %v", err)
	   }
	  // 同 lease_id 第二次写应返回 ErrAlreadyProven
	   proof2 := &invleasegate.InvGateDriftProof{
		  ProofID: "proof-002",
		   LeaseID: "lease-001",
		  Asset:   "BTC",
		   Kind: invleasegate.DriftCheckerDown,
		  State:     invleasegate.ProofOpen,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := store.SaveProof(ctx, proof2); !errors.Is(err, invleasegate.ErrAlreadyProven) {
		  t.Errorf("重复写 proof error = %v, wantErr ErrAlreadyProven", err)
	  }
  }
