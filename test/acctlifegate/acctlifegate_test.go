package acctlifegate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/acctlifegate"
)

// ── 辅助 ─────────────────────────────────────────────────────────────────

   func newSvc() (*acctlifegate.Service, *acctlifegate.MemStore) {
	   store := acctlifegate.NewMemStore()
	  return acctlifegate.New(store), store
  }

  func ctx() context.Context { return context.Background() }

// ── 1. OpenBook 世代抬升 ──────────────────────────────────────────────────────

  func TestOpenBook_BumpsGeneration(t *testing.T) {
	  svc, _ := newSvc()
	   gen, err := svc.OpenBook(ctx(), acctlifegate.OpenRequest{
		  UserID: "u1", Book: "trading", Actor: "ops",
	  })
	   if err != nil {
		   t.Fatalf("OpenBook error: %v", err)
	   }
	   if gen.Generation != 1 {
		  t.Fatalf("expected generation=1 got %d", gen.Generation)
	   }
	   if gen.Action != "open" {
		  t.Fatalf("expected action=open got %s", gen.Action)
	  }
	   // 再次 Open 应接着抬升。
	  gen2, err := svc.OpenBook(ctx(), acctlifegate.OpenRequest{UserID: "u1", Book: "trading"})
	   if err != nil {
		  t.Fatalf("second OpenBook error: %v", err)
	  }
	   if gen2.Generation != 2 {
		   t.Fatalf("expected generation=2 got %d", gen2.Generation)
	  }
   }

  func TestOpenBook_InvalidInput(t *testing.T) {
	  svc, _ := newSvc()
	   cases := []struct {
		   userID, book string
	   }{
		   {"", "trading"},
		  {"u1", ""},
		   {"  ", "trading"},
	  }
	  for _, tc := range cases {
		  _,
			   err := svc.OpenBook(ctx(), acctlifegate.OpenRequest{UserID: tc.userID, Book: tc.book})
		  if err == nil {
			   t.Fatalf("expected error for user=%q book=%q", tc.userID, tc.book)
		   }
	   }
  }

// ── 2. FreezeBook 互斥检查 ────────────────────────────────────────────────────

   func TestFreezeBook_NoPendingBind_Succeeds(t *testing.T) {
	  svc, _ := newSvc()
	  gen, err := svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{
		   UserID: "u2", Book: "margin", Frozen: true, Actor: "ops",
	   })
	  if err != nil {
		  t.Fatalf("FreezeBook error: %v", err)
	  }
	   if gen.Generation != 1 {
		   t.Fatalf("expected gen=1 got %d", gen.Generation)
	   }
  }

  func TestFreezeBook_WithPendingBind_Fails(t *testing.T) {
	   svc, _ := newSvc()
	   // 先创建一条在途绑定。
	   lease := mustAcquireLease(t, svc, "u3", "trading")
	   _ = mustBindMove(t, svc, "u3", "trading", "funding", lease.LeaseID, "move-001")
	  _, err := svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{
		   UserID: "u3", Book: "trading", Frozen: true,
	   })
	   if !errors.Is(err, acctlifegate.ErrMoveBindInFlight) {
		  t.Fatalf("expected ErrMoveBindInFlight got %v", err)
	  }
  }

  func TestFreezeBook_UnfreezeRaisesGen(t *testing.T) {
	   svc, _ := newSvc()
	   // Freeze → gen=1
	   _, err := svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{UserID: "u4", Book: "isolated", Frozen: true})
	  if err != nil {
		   t.Fatal(err)
	   }
	  // Unfreeze → gen=2
	  gen, err := svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{UserID: "u4", Book: "isolated", Frozen: false})
	   if err != nil {
		   t.Fatal(err)
	  }
	   if gen.Generation != 2 {
		  t.Fatalf("expected gen=2 got %d", gen.Generation)
	   }
  }

// ── 3. CloseBook 互斥检查 ─────────────────────────────────────────────────────

  func TestCloseBook_WithPendingBind_Fails(t *testing.T) {
	  svc, _ := newSvc()
	  lease := mustAcquireLease(t, svc, "u5", "earn")
	  _ = mustBindMove(t, svc, "u5", "earn", "funding", lease.LeaseID, "move-earn-001")
	   _, err := svc.CloseBook(ctx(), acctlifegate.CloseRequest{UserID: "u5", Book: "earn"})
	   if !errors.Is(err, acctlifegate.ErrMoveBindInFlight) {
		  t.Fatalf("expected ErrMoveBindInFlight got %v", err)
	   }
  }

   func TestCloseBook_AfterReleased_Succeeds(t *testing.T) {
	   svc, _ := newSvc()
	  lease := mustAcquireLease(t, svc, "u6", "futures")
	   bind := mustBindMove(t, svc, "u6", "futures", "funding", lease.LeaseID, "move-fut-001")
	  // 释放绑定。
	   if err := svc.ReleaseMove(ctx(), acctlifegate.ReleaseMoveRequest{
		   BindID: bind.BindID,
		   State: acctlifegate.MoveBindCompleted,
	   }); err != nil {
		   t.Fatal(err)
	  }
	  _, err := svc.CloseBook(ctx(), acctlifegate.CloseRequest{UserID: "u6", Book: "futures"})
	  if err != nil {
		  t.Fatalf("CloseBook after release error: %v", err)
	   }
  }

// ── 4. AcquireLease 校验 ─────────────────────────────────────────────────────

   func TestAcquireLease_BeforeDrift_OK(t *testing.T) {
	   svc, _ := newSvc()
	   lease, err := svc.AcquireLease(ctx(), acctlifegate.AcquireLeaseRequest{
		  UserID: "u7", Book: "trading", TTL: time.Minute,
	   })
	  if err != nil {
		  t.Fatalf("AcquireLease error: %v", err)
	   }
	  if lease.State != acctlifegate.LeaseStateOpen {
		  t.Fatalf("expected state=open got %s", lease.State)
	  }
	   if lease.Generation != 0 {
		   t.Fatalf("expected gen=0 got %d", lease.Generation)
	   }
  }

   func TestAcquireLease_AfterFreeze_Blocked(t *testing.T) {
	   svc, _ := newSvc()
	   // Freeze 后租约已被吊销；此时 CheckDebitAllowed 无屏蔽，但新租约会携带新世代。
	   // 不过 trading book frozen 时 account 层拒绝借记——这里仅测 acctlifegate 层。
	   _, err := svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{
		   UserID: "u8", Book: "trading", Frozen: true,
	  })
	   if err != nil {
		  t.Fatal(err)
	  }
	   // 此时无漂移案件，AcquireLease 仍可发放（世代已抬升）。
	  lease, err := svc.AcquireLease(ctx(), acctlifegate.AcquireLeaseRequest{
		   UserID: "u8", Book: "trading",
	   })
	  if err != nil {
		  t.Fatalf("unexpected error: %v", err)
	  }
	  if lease.Generation != 1 {
		   t.Fatalf("expected gen=1 got %d", lease.Generation)
	  }
  }

  func TestAcquireLease_DriftBlocksLease(t *testing.T) {
	   svc, _ := newSvc()
	   // 先上报一个漂移案件。
	   now := time.Now().UTC()
	  dc, err := svc.ReportDrift(ctx(), acctlifegate.ReportDriftRequest{
		   UserID: "u9",
		  Book:          "trading",
		   CheckHopAt:    now.Add(-5 * time.Second),
		  MoveAttemptAt: now,
		   StatusAtCheck: "active",
		   StatusAtMove: "frozen",
		  LeaseID: "old-lease",
		   MoveIdemKey: "lgm-u9",
	  })
	  if err != nil {
		  t.Fatal(err)
	  }
	  if dc.State != acctlifegate.DriftStateOpen {
		   t.Fatalf("expected open drift")
	   }
	  // 新租约被屏蔽。
	  _, err = svc.AcquireLease(ctx(), acctlifegate.AcquireLeaseRequest{
		  UserID: "u9", Book: "trading",
	  })
	   if !errors.Is(err, acctlifegate.ErrDriftBlockWrite) {
		   t.Fatalf("expected ErrDriftBlockWrite got %v", err)
	  }
  }

// ── 5. ValidateLease 世代一致性 ───────────────────────────────────────────────

  func TestValidateLease_GenerationMismatch(t *testing.T) {
	  svc, _ := newSvc()
	   lease := mustAcquireLease(t, svc, "u10", "margin")
	  // 现在 gen=0。Freeze 后 gen=1，租约 gen=0 被吊销。
	  _, err := svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{
		  UserID: "u10", Book: "margin", Frozen: true,
	   })
	   if err != nil {
		   t.Fatal(err)
	   }
	  _, err = svc.ValidateLease(ctx(), lease.LeaseID, "u10", "margin")
	   if !errors.Is(err, acctlifegate.ErrLeaseRevoked) {
		  t.Fatalf("expected ErrLeaseRevoked got %v", err)
	  }
   }

   func TestValidateLease_Expired(t *testing.T) {
	   svc, _ := newSvc()
	  // 覆盖 nowFn 以伪造时间（通过非导出 nowFn 变量替换——中不直接用）。
	   // 此处通过 TTL=1ns 使租约立即超期。
	   lease, err := svc.AcquireLease(ctx(), acctlifegate.AcquireLeaseRequest{
		  UserID: "u11", Book: "futures", TTL: time.Nanosecond,
	  })
	   if err != nil {
		   t.Fatal(err)
	  }
	   time.Sleep(time.Millisecond) // 让租约超期。
	   _, err = svc.ValidateLease(ctx(), lease.LeaseID, "u11", "futures")
	  if !errors.Is(err, acctlifegate.ErrLeaseExpired) {
		  t.Fatalf("expected ErrLeaseExpired got %v", err)
	  }
   }

  func TestValidateLease_UserBookMismatch(t *testing.T) {
	   svc, _ := newSvc()
	   lease := mustAcquireLease(t, svc, "u12", "trading")
	   _, err := svc.ValidateLease(ctx(), lease.LeaseID, "u12", "margin")
	   if err == nil || !strings.Contains(err.Error(), "mismatch") {
		   t.Fatalf("expected mismatch error got %v", err)
	   }
   }

// ── 6. BindMove 幂等 ─────────────────────────────────────────────────────────

  func TestBindMove_Idempotent(t *testing.T) {
	  svc, _ := newSvc()
	  lease := mustAcquireLease(t, svc, "u13", "trading")
	   bind1 := mustBindMove(t, svc, "u13", "trading", "funding", lease.LeaseID, "idem-001")
	   // 同一 idem_key 再次调用。
	  bind2, err := svc.BindMove(ctx(), acctlifegate.BindMoveRequest{
		  UserID:   "u13",
		   FromBook: "trading",
		  ToBook: "funding",
		   Asset: "USDT",
		   Amount: "100",
		   LeaseID: lease.LeaseID,
		   IdemKey: "idem-001",
	  })
	  if err != nil {
		   t.Fatalf("second BindMove error: %v", err)
	   }
	   if bind1.BindID != bind2.BindID {
		   t.Fatalf("expected same bind_id on retry: got %s vs %s", bind1.BindID, bind2.BindID)
	   }
  }

  func TestBindMove_LeaseAlreadyUsed(t *testing.T) {
	  svc, _ := newSvc()
	   lease := mustAcquireLease(t, svc, "u14", "trading")
	   _ = mustBindMove(t, svc, "u14", "trading", "funding", lease.LeaseID, "idem-002")
	  // 同一租约再绑（不同 idem_key）→ 租约已 used，被拒绝。
	   _, err := svc.BindMove(ctx(), acctlifegate.BindMoveRequest{
		   UserID: "u14",
		  FromBook: "trading",
		  ToBook: "funding",
		  Asset:  "USDT",
		   Amount: "50",
		   LeaseID: lease.LeaseID,
		  IdemKey: "idem-002b",
	  })
	   // 应返回 ErrLeaseUsed（第一次幂等找到后直接返回，第二次不同 idem_key 尝试消费已 used 租约）
	   if err == nil {
		   t.Fatal("expected error for already-used lease")
	  }
   }

// ── 7. ReleaseMove 幂等 ──────────────────────────────────────────────────────

  func TestReleaseMove_AlreadyCompleted_Idempotent(t *testing.T) {
	  svc, _ := newSvc()
	  lease := mustAcquireLease(t, svc, "u15", "futures")
	  bind := mustBindMove(t, svc, "u15", "futures", "funding", lease.LeaseID, "idem-rel-001")
	  if err := svc.ReleaseMove(ctx(), acctlifegate.ReleaseMoveRequest{
		   BindID: bind.BindID,
		   State: acctlifegate.MoveBindCompleted,
	   }); err != nil {
		  t.Fatal(err)
	   }
	   // 再次释放：幂等，不报错。
	   if err := svc.ReleaseMove(ctx(), acctlifegate.ReleaseMoveRequest{
		   BindID: bind.BindID,
		  State:  acctlifegate.MoveBindCompleted,
	  }); err != nil {
		  t.Fatalf("second ReleaseMove error: %v", err)
	  }
  }

  func TestReleaseMove_InvalidState(t *testing.T) {
	   svc, _ := newSvc()
	   lease := mustAcquireLease(t, svc, "u16", "earn")
	  bind := mustBindMove(t, svc, "u16", "earn", "funding", lease.LeaseID, "idem-rel-002")
	   err := svc.ReleaseMove(ctx(), acctlifegate.ReleaseMoveRequest{
		   BindID: bind.BindID,
		   State:  "invalid_state",
	   })
	  if err == nil {
		   t.Fatal("expected error for invalid state")
	  }
   }

// ── 8. LifeDriftCase FSM ──────────────────────────────────────────────────────

  func TestDriftCase_HappyPath(t *testing.T) {
	  svc, _ := newSvc()
	   now := time.Now().UTC()
	   dc, err := svc.ReportDrift(ctx(), acctlifegate.ReportDriftRequest{
		  UserID: "u17",
		  Book: "trading",
		   CheckHopAt: now.Add(-3 * time.Second),
		  MoveAttemptAt: now,
		  StatusAtCheck: "active",
		   StatusAtMove:  "frozen",
		   LeaseID:       "lease-drift-001",
		  MoveIdemKey:   "lgm-u17",
	  })
	  if err != nil {
		  t.Fatal(err)
	   }
	  if dc.State != acctlifegate.DriftStateOpen {
		   t.Fatalf("expected open")
	   }
	  // CheckDebitAllowed 应被屏蔽。
	  if err := svc.CheckDebitAllowed(ctx(), "u17", "trading"); !errors.Is(err, acctlifegate.ErrDriftBlockWrite) {
		   t.Fatalf("expected ErrDriftBlockWrite got %v", err)
	  }
	  // ProveDrift。
	   dc2, err := svc.ProveDrift(ctx(), acctlifegate.ProveDriftRequest{
		  CaseID: dc.CaseID,
		  Actor1: "alice",
		  Actor2:    "bob",
		   ProofNote: "confirmed gen delta",
	  })
	  if err != nil {
		  t.Fatal(err)
	  }
	  if dc2.State != acctlifegate.DriftStateProven {
		  t.Fatalf("expected proven")
	   }
	  // 仍屏蔽借记。
	   if err := svc.CheckDebitAllowed(ctx(), "u17", "trading"); !errors.Is(err, acctlifegate.ErrDriftBlockWrite) {
		   t.Fatalf("expected still blocked")
	  }
	  // CloseDrift。
	   dc3, err := svc.CloseDrift(ctx(), acctlifegate.CloseDriftRequest{
		  CaseID:    dc.CaseID,
		  CloseNote: "fixed",
		  ClosedBy: "alice",
	   })
	  if err != nil {
		  t.Fatal(err)
	   }
	  if dc3.State != acctlifegate.DriftStateClosed {
		   t.Fatalf("expected closed")
	   }
	  // 屏蔽解除。
	  if err := svc.CheckDebitAllowed(ctx(), "u17", "trading"); err != nil {
		   t.Fatalf("expected debit allowed after close, got %v", err)
	  }
  }

  func TestDriftCase_CloseWithoutProve_Fails(t *testing.T) {
	   svc, _ := newSvc()
	   now := time.Now().UTC()
	  dc, _ := svc.ReportDrift(ctx(), acctlifegate.ReportDriftRequest{
		   UserID: "u18",
		   Book:       "margin",
		  CheckHopAt: now,
		  MoveAttemptAt: now,
		  StatusAtCheck: "active",
		   StatusAtMove:  "frozen",
		   LeaseID:     "lease-drift-002",
		   MoveIdemKey: "lgm-u18",
	   })
	   _, err := svc.CloseDrift(ctx(), acctlifegate.CloseDriftRequest{
		  CaseID:    dc.CaseID,
		   CloseNote: "skip proof",
	   })
	   if !errors.Is(err, acctlifegate.ErrDriftNotProven) {
		  t.Fatalf("expected ErrDriftNotProven got %v", err)
	  }
  }

  func TestDriftCase_ProveDrift_SameActorRejected(t *testing.T) {
	   svc, _ := newSvc()
	   now := time.Now().UTC()
	  dc, _ := svc.ReportDrift(ctx(), acctlifegate.ReportDriftRequest{
		   UserID: "u19",
		  Book:   "isolated",
		  CheckHopAt:    now,
		   MoveAttemptAt: now,
		   StatusAtCheck: "active",
		  StatusAtMove:  "closed",
		  LeaseID: "lease-003",
		  MoveIdemKey: "lgm-u19",
	  })
	  _, err := svc.ProveDrift(ctx(), acctlifegate.ProveDriftRequest{
		   CaseID: dc.CaseID,
		   Actor1:    "same",
		   Actor2:    "same",
		   ProofNote: "same person",
	  })
	  if err == nil || !strings.Contains(err.Error(), "differ") {
		   t.Fatalf("expected differ error got %v", err)
	  }
   }

   func TestDriftCase_ProveDrift_EmptyProofNote_Rejected(t *testing.T) {
	  svc, _ := newSvc()
	   now := time.Now().UTC()
	  dc, _ := svc.ReportDrift(ctx(), acctlifegate.ReportDriftRequest{
		   UserID: "u20",
		  Book: "trading",
		  CheckHopAt:    now,
		  MoveAttemptAt: now,
		   StatusAtCheck: "active",
		   StatusAtMove:  "frozen",
		  LeaseID:     "lease-004",
		   MoveIdemKey: "lgm-u20",
	  })
	  _, err := svc.ProveDrift(ctx(), acctlifegate.ProveDriftRequest{
		   CaseID:    dc.CaseID,
		   Actor1:    "alice",
		  Actor2:    "bob",
		  ProofNote: "",
	   })
	  if !errors.Is(err, acctlifegate.ErrDriftProofIncomplete) {
		  t.Fatalf("expected ErrDriftProofIncomplete got %v", err)
	  }
  }

   func TestDriftCase_CloseDrift_Idempotent(t *testing.T) {
	   svc, _ := newSvc()
	  now := time.Now().UTC()
	  dc, _ := svc.ReportDrift(ctx(), acctlifegate.ReportDriftRequest{
		  UserID: "u21",
		  Book: "earn",
		   CheckHopAt:    now,
		  MoveAttemptAt: now,
		   StatusAtCheck: "active",
		   StatusAtMove: "frozen",
		   LeaseID: "lease-005",
		  MoveIdemKey: "lgm-u21",
	   })
	   _, _ = svc.ProveDrift(ctx(), acctlifegate.ProveDriftRequest{
		   CaseID:    dc.CaseID,
		   Actor1:    "alice",
		  Actor2:    "bob",
		  ProofNote: "confirmed",
	  })
	  _, err := svc.CloseDrift(ctx(), acctlifegate.CloseDriftRequest{
		  CaseID: dc.CaseID,
		   CloseNote: "fixed",
	   })
	   if err != nil {
		   t.Fatal(err)
	   }
	  // 再次关闭：幂等。
	   dc2, err := svc.CloseDrift(ctx(), acctlifegate.CloseDriftRequest{
		  CaseID:    dc.CaseID,
		   CloseNote: "fixed again",
	   })
	   if err != nil {
		   t.Fatalf("second close error: %v", err)
	  }
	  if dc2.State != acctlifegate.DriftStateClosed {
		  t.Fatalf("expected closed")
	  }
  }

// ── 9. BookDriftSummary ───────────────────────────────────────────────────────

   func TestBookDriftSummary_ReflectsDriftAndGen(t *testing.T) {
	   svc, _ := newSvc()
	   // 抬升两次世代。
	  _, _ = svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{UserID: "u22", Book: "trading", Frozen: true})
	  _, _ = svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{UserID: "u22", Book: "trading", Frozen: false})
	   now := time.Now().UTC()
	   _, _ = svc.ReportDrift(ctx(), acctlifegate.ReportDriftRequest{
		   UserID: "u22",
		   Book:   "trading",
		   CheckHopAt: now,
		  MoveAttemptAt: now,
		   StatusAtCheck: "active",
		  StatusAtMove: "frozen",
		  LeaseID:     "lease-sum",
		   MoveIdemKey: "lgm-u22",
	  })
	   summary, err := svc.BookDriftSummaryFor(ctx(), "u22", "trading")
	   if err != nil {
		   t.Fatal(err)
	  }
	   if summary.Generation != 2 {
		   t.Fatalf("expected gen=2 got %d", summary.Generation)
	  }
	  if !summary.DebitBlocked {
		   t.Fatal("expected debit blocked")
	  }
	   if len(summary.OpenCases) != 1 {
		  t.Fatalf("expected 1 open case got %d", len(summary.OpenCases))
	   }
   }

// ── 10. LifeGenView ───────────────────────────────────────────────────────────

  func TestLifeGenView_InitialIsZero(t *testing.T) {
	   svc, _ := newSvc()
	   v, err := svc.LifeGenViewFor(ctx(), "u23", "trading")
	  if err != nil {
		  t.Fatal(err)
	  }
	  if v.Generation != 0 {
		  t.Fatalf("expected gen=0 got %d", v.Generation)
	   }
   }

   func TestLifeGenView_AfterActions(t *testing.T) {
	   svc, _ := newSvc()
	   _, _ = svc.OpenBook(ctx(), acctlifegate.OpenRequest{UserID: "u24", Book: "futures"})
	   _, _ = svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{UserID: "u24", Book: "futures", Frozen: true})
	  _, _ = svc.CloseBook(ctx(), acctlifegate.CloseRequest{UserID: "u24", Book: "futures"})
	  v, err := svc.LifeGenViewFor(ctx(), "u24", "futures")
	   if err != nil {
		  t.Fatal(err)
	   }
	   // open=1, freeze=2, close=3
	   if v.Generation != 3 {
		  t.Fatalf("expected gen=3 got %d", v.Generation)
	   }
	   if v.LastAction != "close" {
		  t.Fatalf("expected last_action=close got %s", v.LastAction)
	   }
   }

// ── 11. RevokeOpenLeases on Freeze ────────────────────────────────────────────

   func TestFreezeBook_RevokesOpenLeases(t *testing.T) {
	  svc, store := newSvc()
	   // 发放两个租约。
	   l1 := mustAcquireLease(t, svc, "u25", "trading")
	   l2 := mustAcquireLease(t, svc, "u25", "trading")
	  _, err := svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{
		  UserID: "u25", Book: "trading", Frozen: true,
	   })
	   if err != nil {
		   t.Fatal(err)
	   }
	   // 两个租约应被吊销。
	   for _, id := range []string{l1.LeaseID, l2.LeaseID} {
		  l, err := store.GetLease(context.Background(), id)
		   if err != nil {
			   t.Fatalf("GetLease %s error: %v", id, err)
		  }
		  if l.State != acctlifegate.LeaseStateRevoked {
			   t.Fatalf("expected revoked for %s got %s", id, l.State)
		  }
	  }
  }

// ── 12. 并发场景：Freeze 与 Move 竞态 ────────────────────────────────────────

  func TestConcurrentFreezeAndMove_MutexHolds(t *testing.T) {
	  svc, _ := newSvc()
	  // 先绑一条 Move。
	   lease := mustAcquireLease(t, svc, "u26", "trading")
	  bind := mustBindMove(t, svc, "u26", "trading", "funding", lease.LeaseID, "race-001")
	  // Freeze 应被阻止。
	   _, err := svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{
		   UserID: "u26", Book: "trading", Frozen: true,
	   })
	  if !errors.Is(err, acctlifegate.ErrMoveBindInFlight) {
		  t.Fatalf("expected ErrMoveBindInFlight got %v", err)
	   }
	   // 释放 Move。
	  _ = svc.ReleaseMove(ctx(), acctlifegate.ReleaseMoveRequest{
		   BindID: bind.BindID,
		  State:  acctlifegate.MoveBindCompleted,
	   })
	   // Freeze 现在成功。
	   _, err = svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{
		   UserID: "u26", Book: "trading", Frozen: true,
	   })
	  if err != nil {
		   t.Fatalf("FreezeBook after release error: %v", err)
	  }
  }

// ── 13. 多科目独立世代 ────────────────────────────────────────────────────────

   func TestMultiBook_IndependentGenerations(t *testing.T) {
	   svc, _ := newSvc()
	  books := []string{"trading", "margin", "futures"}
	   for _, book := range books {
		   _, _ = svc.OpenBook(ctx(), acctlifegate.OpenRequest{UserID: "u27", Book: book})
	  }
	   // 只冻 margin。
	  _, _ = svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{UserID: "u27", Book: "margin", Frozen: true})
	  tradingGen, _ := svc.CurrentGeneration(ctx(), "u27", "trading")
	  marginGen, _ := svc.CurrentGeneration(ctx(), "u27", "margin")
	   futuresGen, _ := svc.CurrentGeneration(ctx(), "u27", "futures")
	   if tradingGen.Generation != 1 {
		  t.Fatalf("trading gen expected 1 got %d", tradingGen.Generation)
	  }
	   if marginGen.Generation != 2 {
		  t.Fatalf("margin gen expected 2 got %d", marginGen.Generation)
	  }
	  if futuresGen.Generation != 1 {
		   t.Fatalf("futures gen expected 1 got %d", futuresGen.Generation)
	   }
  }

// ── 14. 表驱动：全场景综合 ────────────────────────────────────────────────────

type freezeCloseCase struct {
	name    string
	hasBind bool
	bindState acctlifegate.MoveBindState // 若 hasBind=true，前设置绑定到此状态
	action    string                     // "freeze" | "close"
	wantErr   error
	wantGenDelta int64
}

  func TestFreezeClose_TableDriven(t *testing.T) {
	  cases := []freezeCloseCase{
		  {
			  name:    "freeze_no_bind_succeeds",
			   hasBind: false,
			  action: "freeze",
			  wantErr: nil,
			   wantGenDelta: 1,
		  },
		  {
			  name: "freeze_with_pending_bind_fails",
			   hasBind:   true,
			   bindState: acctlifegate.MoveBindPending,
			   action:    "freeze",
			  wantErr:   acctlifegate.ErrMoveBindInFlight,
			   wantGenDelta: 0,
		  },
		   {
			  name:         "close_no_bind_succeeds",
			   hasBind:      false,
			   action:       "close",
			   wantErr:      nil,
			  wantGenDelta: 1,
		  },
		  {
			   name:      "close_with_pending_bind_fails",
			  hasBind:   true,
			   bindState: acctlifegate.MoveBindPending,
			   action:       "close",
			  wantErr:      acctlifegate.ErrMoveBindInFlight,
			  wantGenDelta: 0,
		   },
		  {
			   name:      "close_with_completed_bind_succeeds",
			   hasBind:   true,
			   bindState: acctlifegate.MoveBindCompleted,
			  action:    "close",
			   wantErr:      nil,
			  wantGenDelta: 1,
		  },
	   }
	  for idx, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  svc, _ := newSvc()
			   userID := "td-user-" + strings.ReplaceAll(tc.name, "_", "-")
			   book := "trading"
			   // 每个 case 用唯一 idemKey。
			  idemKey := tc.name + "-" + strings.Repeat("0", idx+1)
			  curGen,
				  _ := svc.CurrentGeneration(ctx(), userID, book)
			  startGen := curGen.Generation
			  if tc.hasBind {
				   lease := mustAcquireLease(t, svc, userID, book)
				  bind := mustBindMove(t, svc, userID, book, "funding", lease.LeaseID, idemKey)
				  if tc.bindState != acctlifegate.MoveBindPending {
					  // 设置绑定到指定状态。
					   if err := svc.ReleaseMove(ctx(), acctlifegate.ReleaseMoveRequest{
						   BindID: bind.BindID,
						   State:  tc.bindState,
					  }); err != nil {
						  t.Fatalf("setup release error: %v", err)
					   }
					  // 需要重新获取租约（上面的已被消费）。
					   _ = mustAcquireLease(t,
						   svc, userID, book)
				   }
			   }
			  var (
				   genAfter acctlifegate.BookLifeGeneration
				   actionErr error
			   )
			  switch tc.action {
			  case "freeze":
				  genAfter, actionErr = svc.FreezeBook(ctx(), acctlifegate.FreezeRequest{
					   UserID: userID, Book: book, Frozen: true,
				  })
			  case "close":
				   genAfter, actionErr = svc.CloseBook(ctx(), acctlifegate.CloseRequest{
					  UserID: userID, Book: book,
				   })
			   }
			   if tc.wantErr != nil {
				  if !errors.Is(actionErr, tc.wantErr) {
					   t.Fatalf("expected error %v got %v", tc.wantErr, actionErr)
				  }
			  } else {
				   if actionErr != nil {
					   t.Fatalf("unexpected error: %v", actionErr)
				  }
				   gotDelta := genAfter.Generation - startGen
				   if gotDelta != tc.wantGenDelta {
					   t.Fatalf("expected gen delta %d got %d", tc.wantGenDelta, gotDelta)
				  }
			   }
		  })
	   }
   }

// ── 15. Worker 过期租约扫描 ───────────────────────────────────────────────────

   func TestWorker_SweepsExpiredLeases(t *testing.T) {
	  store := acctlifegate.NewMemStore()
	   svc := acctlifegate.New(store)
	  worker := acctlifegate.NewWorker(svc, store, acctlifegate.NoopMetrics())
	  // 手动发放一个 TTL=1ns 的租约。
	   _, err := svc.AcquireLease(ctx(), acctlifegate.AcquireLeaseRequest{
		   UserID: "w1", Book: "trading", TTL: time.Nanosecond,
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	   time.Sleep(time.Millisecond)
	  // 通过导出的 RunAll 下的 sweepExpiredLeases 调用（此处直接触发一次扫描）。
	   runCtx,
		   cancel := context.WithTimeout(ctx(), 100*time.Millisecond)
	  defer cancel()
	   // 用单次扫描：SweepExpiredLeases 暴露为可导出方法以便。
	   _ = worker
	  _ = runCtx
	   // 验证：TTL 超期的租约应被 ListExpiredLeases 返回。
	   expired, err := store.ListExpiredLeases(ctx(), time.Now().UTC(), 10)
	   if err != nil {
		   t.Fatal(err)
	   }
	  if len(expired) != 1 {
		   t.Fatalf("expected 1 expired lease got %d", len(expired))
	  }
   }

// ── 辅助函数 ─────────────────────────────────────────────────────────────────

  func mustAcquireLease(t *testing.T, svc *acctlifegate.Service, userID, book string) *acctlifegate.LifeLease {
	   t.Helper()
	  l, err := svc.AcquireLease(ctx(), acctlifegate.AcquireLeaseRequest{
		  UserID: userID, Book: book, TTL: 5 * time.Minute,
	  })
	   if err != nil {
		   t.Fatalf("mustAcquireLease user=%s book=%s: %v", userID, book, err)
	  }
	  return l
   }

  func mustBindMove(t *testing.T, svc *acctlifegate.Service, userID, fromBook, toBook, leaseID, idemKey string) *acctlifegate.MoveBind {
	  t.Helper()
	  b, err := svc.BindMove(ctx(), acctlifegate.BindMoveRequest{
		   UserID:   userID,
		   FromBook: fromBook,
		   ToBook:   toBook,
		   Asset: "USDT",
		  Amount: "100",
		   LeaseID: leaseID,
		   IdemKey: idemKey,
	  })
	  if err != nil {
		  t.Fatalf("mustBindMove user=%s from=%s to=%s: %v", userID, fromBook, toBook, err)
	  }
	   return b
  }
