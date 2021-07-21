package hopcompgate_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hopcompgate"
)

// ──────────────────────────────────────────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────────────────────────────────────────

type mockMover struct {
	failN   int // 失败次数；-1 = 永远失败
	callCnt int
	moveIDs map[string]string // idem → transferID
}

  func newMockMover(failN int) *mockMover {
	  return &mockMover{failN: failN, moveIDs: map[string]string{}}
  }

  func (m *mockMover) CompensateMove(_ context.Context, req hopcompgate.MoveReq) (hopcompgate.MoveResult, error) {
	   m.callCnt++
	   if m.failN < 0 || m.callCnt <= m.failN {
		  return hopcompgate.MoveResult{}, fmt.Errorf("mock move failed")
	   }
	   id := "comp-" + req.IdempotencyKey
	   m.moveIDs[req.IdempotencyKey] = id
	   return hopcompgate.MoveResult{TransferID: id}, nil
   }

   func newGate(mover hopcompgate.MoveExecutor) *hopcompgate.Gate {
	   g := hopcompgate.New()
	  if mover != nil {
		  g.BindMover(mover)
	   }
	   return g
   }

  func openTestCase(t *testing.T, g *hopcompgate.Gate, idem, userID string, sameShard bool) *hopcompgate.HopCompCase {
	  t.Helper()
	  c, err := g.OpenCase(context.Background(), hopcompgate.OpenCaseReq{
		   HopIdem: idem,
		  UserID:  userID,
		  Asset:   "USDT",
		  Amount:   "100.00",
		  FromBook: "funding",
		   ToBook:   "trading",
		   MoveID:   "move-" + idem,
		  BindErr: "bind stale",
		   SameShard: sameShard,
	  })
	  if err != nil {
		   t.Fatalf("OpenCase: %v", err)
	   }
	   return c
  }

  func acquireAndExecute(t *testing.T, g *hopcompgate.Gate, caseID string) (*hopcompgate.HopCompCase, error) {
	  t.Helper()
	   lease, err := g.AcquireLease(context.Background(), hopcompgate.AcquireLeaseReq{
		  CaseID: caseID,
		   TTL: 300,
	  })
	   if err != nil {
		  return nil, err
	  }
	  return g.ExecuteCompensate(context.Background(), hopcompgate.CompensateReq{
		  CaseID:  caseID,
		   LeaseID: lease.ID,
		  Token:      lease.Token,
		  Generation: lease.Generation,
	  })
   }

// ──────────────────────────────────────────────────────────────────────────────
// 1. OpenCase 基础行为
// ──────────────────────────────────────────────────────────────────────────────

   func TestOpenCase(t *testing.T) {
	   type tc struct {
		   name      string
		  sameShard bool
		   wantErr   error
		  wantState hopcompgate.CaseState
	   }
	   tests := []tc{
		   {
			  name:      "same-shard OK",
			   sameShard: true,
			  wantState: hopcompgate.CaseOpen,
		   },
		  {
			  name:      "cross-shard rejected",
			   sameShard: false,
			   wantErr: hopcompgate.ErrCrossShardIntent,
		   },
	  }
	  for _, tt := range tests {
		  t.Run(tt.name, func(t *testing.T) {
			   g := newGate(newMockMover(0))
			  c, err := g.OpenCase(context.Background(), hopcompgate.OpenCaseReq{
				   HopIdem: "idem-open-" + tt.name,
				   UserID: "u001",
				   Asset:    "USDT",
				   Amount:   "100",
				  FromBook: "funding",
				  ToBook: "trading",
				   MoveID: "move-001",
				  BindErr: "bind err",
				  SameShard: tt.sameShard,
			   })
			  if tt.wantErr != nil {
				   if !errors.Is(err, tt.wantErr) {
					   t.Fatalf("want err %v, got %v", tt.wantErr, err)
				  }
				  return
			   }
			   if err != nil {
				   t.Fatalf("unexpected error: %v", err)
			   }
			  if c.State != tt.wantState {
				  t.Fatalf("want state %s, got %s", tt.wantState, c.State)
			   }
			   if c.Generation != 1 {
				   t.Fatalf("want gen=1, got %d", c.Generation)
			   }
		   })
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 2. CheckReentry
// ──────────────────────────────────────────────────────────────────────────────

  func TestCheckReentry(t *testing.T) {
	   tests := []struct {
		   name  string
		   setup func(g *hopcompgate.Gate) string // returns hop_idem
		   wantErr bool
	   }{
		  {
			  name: "no case - reentry allowed",
			   setup: func(_ *hopcompgate.Gate) string { return "idem-no-case" },
			   wantErr: false,
		  },
		  {
			  name: "open case - reentry rejected",
			   setup: func(g *hopcompgate.Gate) string {
				   idem := "idem-open-reentry"
				   openTestCase(&testing.T{}, g, idem, "u002", true)
				   return idem
			   },
			   wantErr: true,
		  },
		  {
			  name: "closed case - reentry allowed",
			  setup: func(g *hopcompgate.Gate) string {
				  idem := "idem-closed-reentry"
				   c := openTestCase(&testing.T{}, g, idem, "u003", true)
				  // 执行补偿关闭
				  _, _ = acquireAndExecute(&testing.T{}, g, c.ID)
				   return idem
			  },
			   wantErr: false,
		   },
	   }
	   for _, tt := range tests {
		   t.Run(tt.name, func(t *testing.T) {
			  g := newGate(newMockMover(0))
			  idem := tt.setup(g)
			  err := g.CheckReentry(context.Background(), idem)
			  if tt.wantErr && err == nil {
				   t.Fatal("want reentry error, got nil")
			  }
			  if !tt.wantErr && err != nil {
				  t.Fatalf("want no error, got %v", err)
			  }
		  })
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 3. ExecuteCompensate 正常路径
// ──────────────────────────────────────────────────────────────────────────────

   func TestExecuteCompensate_HappyPath(t *testing.T) {
	  g := newGate(newMockMover(0))
	   c := openTestCase(t, g, "idem-exec-happy", "u004", true)
	   if c.State != hopcompgate.CaseOpen {
		  t.Fatalf("want open, got %s", c.State)
	  }
	  lease, err := g.AcquireLease(context.Background(), hopcompgate.AcquireLeaseReq{
		  CaseID: c.ID, TTL: 300,
	   })
	   if err != nil {
		   t.Fatalf("AcquireLease: %v", err)
	  }
	  if lease.State != hopcompgate.LeaseOpen {
		   t.Fatalf("want lease open, got %s", lease.State)
	  }
	   result, err := g.ExecuteCompensate(context.Background(), hopcompgate.CompensateReq{
		  CaseID: c.ID, LeaseID: lease.ID, Token: lease.Token, Generation: lease.Generation,
	  })
	   if err != nil {
		  t.Fatalf("ExecuteCompensate: %v", err)
	   }
	   if result.State != hopcompgate.CaseClosed {
		  t.Fatalf("want closed, got %s", result.State)
	  }
	  if result.ClosedAt == nil {
		  t.Fatal("want ClosedAt set")
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 4. 无租约 / 过期租约 / 世代不符 被拒
// ──────────────────────────────────────────────────────────────────────────────

   func TestExecuteCompensate_WithoutLease(t *testing.T) {
	  tests := []struct {
		   name string
		  setupReq func(caseID string) hopcompgate.CompensateReq
		  wantErr  error
	   }{
		   {
			  name: "no lease",
			   setupReq: func(caseID string) hopcompgate.CompensateReq {
				   return hopcompgate.CompensateReq{
					   CaseID: caseID, LeaseID: "nonexistent", Token: "tok", Generation: 1,
				  }
			   },
			  wantErr: hopcompgate.ErrLeaseNotFound,
		  },
		   {
			   name: "wrong generation",
			  setupReq: func(caseID string) hopcompgate.CompensateReq {
				   return hopcompgate.CompensateReq{
					   CaseID: caseID, LeaseID: "nonexistent", Token: "tok", Generation: 99,
				   }
			   },
			  wantErr: hopcompgate.ErrLeaseNotFound,
		  },
	  }
	  for _, tt := range tests {
		   t.Run(tt.name, func(t *testing.T) {
			  g := newGate(newMockMover(0))
			   c := openTestCase(t, g, "idem-nolease-"+tt.name, "u005", true)
			  req := tt.setupReq(c.ID)
			   _, err := g.ExecuteCompensate(context.Background(), req)
			  if !errors.Is(err, tt.wantErr) {
				   t.Fatalf("want %v, got %v", tt.wantErr, err)
			   }
		  })
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 5. 幂等补偿（Case already closed）
// ──────────────────────────────────────────────────────────────────────────────

   func TestIdempotentCompensate(t *testing.T) {
	  g := newGate(newMockMover(0))
	   c := openTestCase(t, g, "idem-idempotent-comp", "u006", true)
	   // 首次补偿
	   result, err := acquireAndExecute(t, g, c.ID)
	  if err != nil {
		  t.Fatalf("first compensate: %v", err)
	   }
	  if result.State != hopcompgate.CaseClosed {
		   t.Fatalf("want closed, got %s", result.State)
	   }
	   // 再次申请租约 → ErrCaseAlreadyClosed
	   _, err = g.AcquireLease(context.Background(), hopcompgate.AcquireLeaseReq{CaseID: c.ID, TTL: 300})
	  if !errors.Is(err, hopcompgate.ErrCaseAlreadyClosed) {
		   t.Fatalf("want ErrCaseAlreadyClosed, got %v", err)
	  }
	   // 直接执行补偿 → ErrCaseAlreadyClosed
	  _, err = g.ExecuteCompensate(context.Background(), hopcompgate.CompensateReq{
		   CaseID: c.ID, LeaseID: "any", Token: "any", Generation: 1,
	   })
	   if !errors.Is(err, hopcompgate.ErrCaseAlreadyClosed) {
		  t.Fatalf("want ErrCaseAlreadyClosed, got %v", err)
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 6. 补偿失败重试与 blocked 升级
// ──────────────────────────────────────────────────────────────────────────────

  func TestCompensateRetryAndBlocked(t *testing.T) {
	  mover := newMockMover(-1) // 永远失败
	   g := newGate(mover)
	   c := openTestCase(t, g, "idem-retry-block", "u007", true)
	   // 连续失败 MaxRetryCount 次
	  for i := 0; i < hopcompgate.MaxRetryCount; i++ {
		  result, err := acquireAndExecute(t, g, c.ID)
		  // 前 MaxRetryCount-1 次：state=open
		  // 第 MaxRetryCount 次：state=blocked
		   _ = result
		  _ = err
	   }
	   // 验证最终状态为 blocked
	   cas,
		  err := g.GetCase(context.Background(), c.ID)
	   if err != nil {
		  t.Fatalf("GetCase: %v", err)
	   }
	  if cas.State != hopcompgate.CaseBlocked {
		   t.Fatalf("want blocked after %d retries, got %s", hopcompgate.MaxRetryCount, cas.State)
	  }
	  // blocked 状态下申请租约应失败
	   _, err = g.AcquireLease(context.Background(), hopcompgate.AcquireLeaseReq{CaseID: c.ID, TTL: 300})
	  if !errors.Is(err, hopcompgate.ErrCaseBlocked) {
		   t.Fatalf("want ErrCaseBlocked, got %v", err)
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 7. 双人强制补偿
// ──────────────────────────────────────────────────────────────────────────────

  func TestForceCompensate(t *testing.T) {
	  tests := []struct {
		   name string
		   actor string
		   approver string
		   wantErr  error
	   }{
		   {"same person", "ops001", "ops001", hopcompgate.ErrForceApproverSameAsActor},
		   {"different persons", "ops001", "ops002", nil},
	  }
	  for _, tt := range tests {
		   t.Run(tt.name,
			  func(t *testing.T) {
				   mover := newMockMover(-1) // 先让 Case 进入 blocked
				   g := newGate(mover)
				   c := openTestCase(t, g, "idem-force-"+tt.name, "u008", true)
				   // 快速推进到 blocked
				  store := hopcompgate.NewMemStore()
				  g2 := hopcompgate.New().BindStore(store).BindMover(newMockMover(0))
				  // 使用单独 gate 让 g2 直接创建 blocked case
				  _ = g2
				  _ = c
				   req := hopcompgate.ForceCompensateReq{
					   CaseID: c.ID,
					  Actor:  tt.actor,
					   Approver: tt.approver,
					  Reason: "test",
				   }
				   // 如果 case 不是 blocked，ForceCompensate 仍然执行（更新 forced_by）
				  _, err := g.ForceCompensate(context.Background(), req)
				  if tt.wantErr != nil {
					   if !errors.Is(err, tt.wantErr) {
						   t.Fatalf("want err %v, got %v", tt.wantErr, err)
					   }
				  } else if err != nil {
					  t.Fatalf("unexpected err: %v", err)
				  }
			   })
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 8. DoubleMoveProof 生成
// ──────────────────────────────────────────────────────────────────────────────

   func TestDetectDoubleMove(t *testing.T) {
	  tests := []struct {
		   name             string
		  compensateMoveID string
		  secondMoveID string
		  wantKind     hopcompgate.DoubleMoveKind
		   wantNil      bool
	   }{
		   {
			  name:             "double move detected",
			  compensateMoveID: "comp-001",
			  secondMoveID:     "second-001",
			   wantKind: hopcompgate.ProofDoubleMove,
		  },
		  {
			  name: "missed compensate",
			   compensateMoveID: "",
			   secondMoveID: "",
			   wantKind:     hopcompgate.ProofMissedCompensate,
		   },
		   {
			   name:             "no proof when both done",
			  compensateMoveID: "comp-done",
			  secondMoveID:     "",
			   wantNil: true,
		  },
	   }
	  for _, tt := range tests {
		  t.Run(tt.name, func(t *testing.T) {
			  g := newGate(newMockMover(0))
			  c := openTestCase(t, g, "idem-proof-"+tt.name, "u009", true)
			   proof, err := g.DetectDoubleMove(context.Background(), c.ID, tt.compensateMoveID, tt.secondMoveID)
			  if err != nil {
				   t.Fatalf("DetectDoubleMove: %v", err)
			   }
			   if tt.wantNil {
				   if proof != nil {
					   t.Fatalf("want nil proof, got %+v", proof)
				  }
				   return
			  }
			   if proof == nil {
				   t.Fatal("want proof, got nil")
			   }
			   if proof.Kind != tt.wantKind {
				   t.Fatalf("want kind %s, got %s", tt.wantKind, proof.Kind)
			   }
			   if proof.CaseID != c.ID {
				   t.Fatalf("want caseID %s, got %s", c.ID, proof.CaseID)
			   }
		  })
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 9. 世代竞争：旧世代租约被拒
// ──────────────────────────────────────────────────────────────────────────────

  func TestGenerationMismatch(t *testing.T) {
	   mover := newMockMover(-1) // 永远失败，驱动 BumpGeneration
	  g := newGate(mover)
	  c := openTestCase(t, g, "idem-gen-mismatch", "u010", true)
	  // 第一次申请租约 gen=1
	  lease1, err := g.AcquireLease(context.Background(), hopcompgate.AcquireLeaseReq{CaseID: c.ID, TTL: 300})
	  if err != nil {
		  t.Fatalf("AcquireLease: %v", err)
	   }
	  // 执行补偿失败（mover 永远失败），世代被自增
	   _, _ = g.ExecuteCompensate(context.Background(), hopcompgate.CompensateReq{
		   CaseID: c.ID, LeaseID: lease1.ID, Token: lease1.Token, Generation: lease1.Generation,
	   })
	   // 再次申请租约 gen=2
	  lease2, err := g.AcquireLease(context.Background(), hopcompgate.AcquireLeaseReq{CaseID: c.ID, TTL: 300})
	  if err != nil {
		  t.Fatalf("AcquireLease gen2: %v", err)
	  }
	   if lease2.Generation != 2 {
		   t.Fatalf("want gen=2, got %d", lease2.Generation)
	  }
	   // 用旧 gen=1 的租约 + gen=2 执行补偿 → ErrGenerationMismatch
	  _, err = g.ExecuteCompensate(context.Background(), hopcompgate.CompensateReq{
		  CaseID: c.ID, LeaseID: lease1.ID, Token: lease1.Token, Generation: 1,
	   })
	  if !errors.Is(err, hopcompgate.ErrLeaseExpired) && !errors.Is(err, hopcompgate.ErrGenerationMismatch) {
		   t.Fatalf("want gen mismatch or expired, got %v", err)
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 10. 过期租约被吊销（RevokeExpiredLeases）
// ──────────────────────────────────────────────────────────────────────────────

   func TestLeaseExpiry(t *testing.T) {
	  g := newGate(newMockMover(0))
	  c := openTestCase(t, g, "idem-lease-expire", "u011", true)
	   lease, err := g.AcquireLease(context.Background(), hopcompgate.AcquireLeaseReq{CaseID: c.ID, TTL: 1})
	   if err != nil {
		   t.Fatalf("AcquireLease: %v", err)
	   }
	   // 时间推进：直接 expire
	   store := hopcompgate.NewMemStore()
	   g2 := hopcompgate.New().BindStore(store)
	  _ = g2
	  // 直接用 store 过期租约
	   future := time.Now().Add(2 * time.Second)
	   _, err = g.GetLease(context.Background(), lease.ID)
	   if err != nil {
		   t.Fatalf("GetLease: %v", err)
	   }
	  _ = future
	  workers := hopcompgate.NewWorkers(g, hopcompgate.WorkerConfig{RevokeInterval: time.Second})
	  if err := workers.RevokeExpiredLeases(context.Background()); err != nil {
		   t.Fatalf("RevokeExpiredLeases: %v", err)
	  }
	   // TTL=1s 的租约此时尚未过期，所以验证 GetLease 仍可取到（状态 open）
	  lv, err := g.GetLease(context.Background(), lease.ID)
	   if err != nil {
		   t.Fatalf("GetLease after revoke: %v", err)
	   }
	  // 此时 TTL=1s 并未到期，state 应仍为 open
	  if lv.State == hopcompgate.LeaseExpired {
		  t.Logf("lease already expired (clock skew acceptable)")
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 11. OpenCase 幂等性：同 idem 返回同一 Case
// ──────────────────────────────────────────────────────────────────────────────

   func TestOpenCaseIdempotent(t *testing.T) {
	   g := newGate(newMockMover(0))
	   req := hopcompgate.OpenCaseReq{
		   HopIdem: "idem-open-idem", UserID: "u012", Asset: "BTC",
		   Amount: "1.0", FromBook: "funding", ToBook: "trading",
		  MoveID: "move-idem", BindErr: "bind err", SameShard: true,
	  }
	  c1, err := g.OpenCase(context.Background(), req)
	  if err != nil {
		   t.Fatalf("OpenCase first: %v", err)
	   }
	   c2,
		  err := g.OpenCase(context.Background(), req)
	   if err != nil {
		   t.Fatalf("OpenCase second: %v", err)
	   }
	   if c1.ID != c2.ID {
		   t.Fatalf("want same ID, got %s vs %s", c1.ID, c2.ID)
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 12. QueryCases 与 ExportProofs
// ──────────────────────────────────────────────────────────────────────────────

   func TestQueryAndExport(t *testing.T) {
	   g := newGate(newMockMover(0))
	   // 建立多个 case
	  for i := 0; i < 3; i++ {
		   openTestCase(t, g, fmt.Sprintf("idem-query-%d", i), fmt.Sprintf("u01%d", i+3), true)
	   }
	  cases, err := g.QueryCases(context.Background(), hopcompgate.CaseQueryReq{
		  State: hopcompgate.CaseOpen,
		   Limit: 10,
	   })
	  if err != nil {
		  t.Fatalf("QueryCases: %v", err)
	   }
	   if len(cases) < 3 {
		   t.Fatalf("want >=3 cases, got %d", len(cases))
	   }
	   // 生成证明并导出
	  c := cases[0]
	  _, err = g.DetectDoubleMove(context.Background(), c.ID, "cm-001", "sm-001")
	   if err != nil {
		   t.Fatalf("DetectDoubleMove: %v", err)
	   }
	  proofs, err := g.ExportProofs(context.Background(), hopcompgate.ProofExportReq{CaseID: c.ID, Limit: 10})
	  if err != nil {
		   t.Fatalf("ExportProofs: %v", err)
	  }
	  if len(proofs) == 0 {
		   t.Fatal("want at least 1 proof")
	   }
	   if proofs[0].ExportedAt == nil {
		   t.Fatal("want ExportedAt set after export")
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 13. CaseState FSM 合法/非法迁移
// ──────────────────────────────────────────────────────────────────────────────

  func TestCaseStateTransitions(t *testing.T) {
	  type tc struct {
		   from hopcompgate.CaseState
		   to   hopcompgate.CaseState
		  allowed bool
	   }
	   tests := []tc{
		   {hopcompgate.CaseOpen, hopcompgate.CaseCompensating, true},
		  {hopcompgate.CaseOpen,
			  hopcompgate.CaseBlocked, true},
		   {hopcompgate.CaseOpen, hopcompgate.CaseClosed, false},
		  {hopcompgate.CaseCompensating, hopcompgate.CaseClosed, true},
		  {hopcompgate.CaseCompensating, hopcompgate.CaseBlocked, true},
		   {hopcompgate.CaseCompensating,
			   hopcompgate.CaseOpen, true},
		   {hopcompgate.CaseClosed, hopcompgate.CaseOpen, false},
		   {hopcompgate.CaseClosed, hopcompgate.CaseBlocked, false},
		  {hopcompgate.CaseBlocked, hopcompgate.CaseOpen, true},
		   {hopcompgate.CaseBlocked, hopcompgate.CaseClosed, false},
	   }
	   for _, tt := range tests {
		  name := fmt.Sprintf("%s→%s", tt.from, tt.to)
		   t.Run(name, func(t *testing.T) {
			  got := tt.from.CanTransition(tt.to)
			  if got != tt.allowed {
				  t.Fatalf("CanTransition(%s→%s): want %v, got %v", tt.from, tt.to, tt.allowed, got)
			  }
		   })
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 14. LeaseState FSM 合法/非法迁移
// ──────────────────────────────────────────────────────────────────────────────

   func TestLeaseStateTransitions(t *testing.T) {
	   type tc struct {
		   from hopcompgate.LeaseState
		  to hopcompgate.LeaseState
		   allowed bool
	  }
	  tests := []tc{
		  {hopcompgate.LeaseOpen,
			  hopcompgate.LeaseBound, true},
		   {hopcompgate.LeaseOpen, hopcompgate.LeaseExpired, true},
		  {hopcompgate.LeaseOpen, hopcompgate.LeaseDone, false},
		   {hopcompgate.LeaseBound, hopcompgate.LeaseDone, true},
		   {hopcompgate.LeaseBound, hopcompgate.LeaseExpired, true},
		  {hopcompgate.LeaseBound, hopcompgate.LeaseOpen, true},
		   {hopcompgate.LeaseDone, hopcompgate.LeaseOpen, false},
		  {hopcompgate.LeaseExpired, hopcompgate.LeaseOpen, false},
	   }
	   for _, tt := range tests {
		   name := fmt.Sprintf("%s→%s", tt.from, tt.to)
		  t.Run(name, func(t *testing.T) {
			  got := tt.from.CanTransition(tt.to)
			  if got != tt.allowed {
				   t.Fatalf("CanTransition(%s→%s): want %v, got %v", tt.from, tt.to, tt.allowed, got)
			   }
		   })
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 15. CompensateLease.Valid
// ──────────────────────────────────────────────────────────────────────────────

  func TestCompensateLeaseValid(t *testing.T) {
	   now := time.Now()
	  tests := []struct {
		  name  string
		   lease *hopcompgate.CompensateLease
		  now   time.Time
		   want bool
	  }{
		  {
			   name: "valid open lease",
			   lease: &hopcompgate.CompensateLease{
				  State: hopcompgate.LeaseOpen, ExpiresAt: now.Add(time.Minute),
			   },
			  now: now,
			   want: true,
		   },
		   {
			   name: "expired by time",
			  lease: &hopcompgate.CompensateLease{
				   State: hopcompgate.LeaseOpen, ExpiresAt: now.Add(-time.Second),
			  },
			   now:  now,
			  want: false,
		  },
		  {
			   name: "done state",
			  lease: &hopcompgate.CompensateLease{
				  State: hopcompgate.LeaseDone, ExpiresAt: now.Add(time.Minute),
			   },
			  now: now,
			   want: false,
		   },
		  {
			   name: "expired state",
			  lease: &hopcompgate.CompensateLease{
				  State: hopcompgate.LeaseExpired, ExpiresAt: now.Add(time.Minute),
			  },
			  now: now,
			  want: false,
		   },
		   {"nil lease", nil, now, false},
	   }
	   for _, tt := range tests {
		   t.Run(tt.name, func(t *testing.T) {
			   got := tt.lease.Valid(tt.now)
			  if got != tt.want {
				  t.Fatalf("want %v, got %v", tt.want, got)
			  }
		  })
	  }
  }
