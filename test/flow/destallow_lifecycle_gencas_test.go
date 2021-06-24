package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
)

// TestGenAlignment_Match 世代一致时 VerifyGenAlignment 返回 nil。

   func TestGenAlignment_Match(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	  clk := &fakeClock{t: time.Now().UTC()}
	   calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	  cal := destlife.NewCalendar(calStore, auditStore, nil, 0, clk.Now)
	   e, _ := cal.InitAdded(ctx, "user30", "BTC", "bc1genmatch", "", 7)
	  err := cal.VerifyGenAlignment(ctx, e.EntryLifeID, 7)
	  if err != nil {
		   t.Errorf("expected nil for matching gen, got=%v", err)
	  }
  }

// TestGenAlignment_Mismatch 世代不一致时返回 *GenMismatch 错误。

   func TestGenAlignment_Mismatch(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	   clk := &fakeClock{t: time.Now().UTC()}
	   calStore := destlife.NewMemCalendarStore()
	   auditStore := destlife.NewMemAuditStore()
	   cal := destlife.NewCalendar(calStore, auditStore, nil, 0, clk.Now)
	  e, _ := cal.InitAdded(ctx, "user31", "ETH", "0xGenMismatch", "", 3)
	   err := cal.VerifyGenAlignment(ctx, e.EntryLifeID, 99) // book_gen=99 ≠ life_gen=3
	   if err == nil {
		   t.Fatal("expected non-nil error for gen mismatch")
	   }
	   var mm *destlife.GenMismatch
	   if !errors.As(err, &mm) {
		  t.Fatalf("expected *GenMismatch, got=%T %v", err, err)
	   }
	  if mm.LifeGen != 3 || mm.BookGen != 99 {
		  t.Errorf("expected life_gen=3 book_gen=99, got life_gen=%d book_gen=%d", mm.LifeGen, mm.BookGen)
	  }
	  if mm.Discrepancy != 96 {
		   t.Errorf("expected discrepancy=96, got=%d", mm.Discrepancy)
	  }
  }

// TestLifecycleGate_StrictClose GenCAS 世代不一致时 LifecycleGate 关闸并写入 bypass 证明。
  func TestLifecycleGate_StrictClose(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	  clk := &fakeClock{t: time.Now().UTC()}
	   calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	   cal := destlife.NewCalendar(calStore, auditStore, nil, 0, clk.Now)
	  inflightStore := destlife.NewMemInflightStore()
	   lock := destlife.NewInflightLock(inflightStore,
		  cal, clk.Now)
	  bypassStore := destlife.NewMemBypassStore()
	   guard := destlife.NewBypassGuard(bypassStore,
		   cal, clk.Now, 0)
	  e, _ := cal.InitAdded(ctx, "user32", "BTC", "bc1strict", "", 5) // life_gen=5
	   // book_gen=99（不匹配），LifecycleGate 应关闸。
	  err := destlife.LifecycleGate(ctx, cal, guard, lock, e.EntryLifeID, "user32", "BTC", "bc1strict", 99)
	   if err == nil {
		  t.Fatal("expected error: gen mismatch should close gate")
	   }
	  var mm *destlife.GenMismatch
	   if !errors.As(err, &mm) {
		   t.Fatalf("expected *GenMismatch, got=%T %v", err, err)
	   }
	  // 验证 bypass proof 已写入（gen_mismatch）。
	  proofs, _ := guard.ListProofs(ctx, e.EntryLifeID, 10)
	  if len(proofs) == 0 {
		   t.Fatal("expected bypass proof for gen mismatch")
	   }
	  if proofs[0].Reason != destlife.ReasonGenMismatch {
		   t.Errorf("expected reason=gen_mismatch, got=%s", proofs[0].Reason)
	  }
  }

// TestLifecycleGate_CooldownBlock 冷却未满时 LifecycleGate 关闸。
   func TestLifecycleGate_CooldownBlock(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	  clk := &fakeClock{t: time.Now().UTC()}
	  calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	  window := 24 * time.Hour
	   cal := destlife.NewCalendar(calStore, auditStore, nil, window, clk.Now)
	   inflightStore := destlife.NewMemInflightStore()
	  lock := destlife.NewInflightLock(inflightStore, cal, clk.Now)
	   bypassStore := destlife.NewMemBypassStore()
	  guard := destlife.NewBypassGuard(bypassStore, cal, clk.Now, 0)
	  e, _ := cal.InitAdded(ctx, "user33", "ETH", "0xGateBlock", "", 1)
	   // 冷却未满，world_gen=life_gen=1（一致），但仍因冷却被关闸。
	  err := destlife.LifecycleGate(ctx, cal, guard, lock, e.EntryLifeID, "user33", "ETH", "0xGateBlock", 1)
	   if !errors.Is(err, destlife.ErrCooldownActive) {
		  t.Errorf("expected ErrCooldownActive, got=%v", err)
	   }
   }

// TestLifecycleGate_Pass 世代一致且冷却满足时 LifecycleGate 放行。
  func TestLifecycleGate_Pass(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	  clk := &fakeClock{t: time.Now().UTC()}
	  calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	   window := 1 * time.Hour
	  cal := destlife.NewCalendar(calStore, auditStore, nil, window, clk.Now)
	   inflightStore := destlife.NewMemInflightStore()
	   lock := destlife.NewInflightLock(inflightStore,
		   cal, clk.Now)
	   bypassStore := destlife.NewMemBypassStore()
	  guard := destlife.NewBypassGuard(bypassStore,
		  cal, clk.Now, 0)
	  e, _ := cal.InitAdded(ctx, "user34", "BTC", "bc1pass", "", 1)
	   clk.Advance(61 * time.Minute)
	  activated, _ := cal.ActivateCooled(ctx, e.EntryLifeID, clk.Now(), 1)
	   // 世代一致（life_gen=activated.Generation），冷却满足 → 放行。
	  err := destlife.LifecycleGate(ctx, cal, guard, lock, e.EntryLifeID, "user34", "BTC", "bc1pass", activated.Generation)
	   if err != nil {
		  t.Errorf("expected nil (gate pass), got=%v", err)
	  }
   }

// TestStateMachineTransitions 验证全部合法/非法状态转换。

  func TestStateMachineTransitions(t *testing.T) {
	  t.Parallel()
	   type tcase struct {
		   from    destlife.EntryState
		   to      destlife.EntryState
		  wantErr bool
	  }
	  cases := []tcase{
		   {destlife.StateAdded, destlife.StateCooling, false},
		   {destlife.StateCooling, destlife.StateActive, false},
		   {destlife.StateCooling, destlife.StateRevoking, false},
		  {destlife.StateActive, destlife.StateRevoking, false},
		  {destlife.StateRevoking, destlife.StateRevoked, false},
		   // 非法。
		   {destlife.StateAdded, destlife.StateActive, true},
		   {destlife.StateAdded, destlife.StateRevoked, true},
		  {destlife.StateActive, destlife.StateCooling, true},
		   {destlife.StateRevoked, destlife.StateActive, true},
		   {destlife.StateRevoked, destlife.StateCooling, true},
	   }
	  for _, tc := range cases {
		  err := destlife.Transition(tc.from, tc.to)
		   gotErr := err != nil
		  if gotErr != tc.wantErr {
			   t.Errorf("Transition(%s→%s): wantErr=%v got=%v (%v)",
				   tc.from, tc.to, tc.wantErr, gotErr, err)
		  }
	   }
   }
// temporary guard removed after the production check
