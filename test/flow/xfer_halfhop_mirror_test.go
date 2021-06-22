package flow

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/xfermirror"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

  func newTestMirrorMachine() (*xfermirror.Machine, *xfermirror.MemMirrorStore) {
	  store := xfermirror.NewMemMirrorStore()
	  return xfermirror.New(store), store
   }

// TestMirror_CreateProcessing 创建镜像后初始状态为 processing。
   func TestMirror_CreateProcessing(t *testing.T) {
	  machine, _ := newTestMirrorMachine()
	  ctx := context.Background()
	  hopID := uuid.NewString()
	   mv, err := machine.Create(ctx, hopID, "user-a", "user-b", "USDT", "funding", "trading",
		   decimal.NewFromInt(100))
	  if err != nil {
		   t.Fatalf("Create: %v", err)
	   }
	  if mv.State != xfermirror.MirrorProcessing {
		  t.Errorf("expected processing, got %s", mv.State)
	  }
	   if mv.HopID != hopID {
		  t.Error("hop_id mismatch")
	   }
	   if mv.Version != 0 {
		  t.Errorf("expected version 0, got %d", mv.Version)
	  }
  }

// TestMirror_MarkCredited 补入腿后镜像转 credited。
  func TestMirror_MarkCredited(t *testing.T) {
	  machine, _ := newTestMirrorMachine()
	  ctx := context.Background()
	   hopID := uuid.NewString()
	   _, _ = machine.Create(ctx, hopID, "user-a", "user-b", "USDT", "funding", "earn",
		   decimal.NewFromInt(500))
	  mv, err := machine.MarkCredited(ctx, hopID, "entry-001", "plan-001")
	   if err != nil {
		  t.Fatalf("MarkCredited: %v", err)
	  }
	   if mv.State != xfermirror.MirrorCredited {
		   t.Errorf("expected credited, got %s", mv.State)
	   }
	   if mv.EntryRef != "entry-001" {
		   t.Errorf("entry_ref mismatch")
	  }
	   if mv.Version != 1 {
		  t.Errorf("expected version 1, got %d", mv.Version)
	   }
  }

// TestMirror_MarkRolledBack 回滚出腿后镜像转 rolled_back。

   func TestMirror_MarkRolledBack(t *testing.T) {
	   machine, _ := newTestMirrorMachine()
	  ctx := context.Background()
	   hopID := uuid.NewString()
	  _, _ = machine.Create(ctx, hopID, "user-c", "user-d", "BTC", "funding", "trading",
		  decimal.NewFromFloat(0.05))
	   mv, err := machine.MarkRolledBack(ctx, hopID, "rb-entry-002", "plan-002")
	   if err != nil {
		  t.Fatalf("MarkRolledBack: %v", err)
	  }
	   if mv.State != xfermirror.MirrorRolledBack {
		  t.Errorf("expected rolled_back, got %s", mv.State)
	   }
  }

// TestMirror_MarkManual 人工强制推进到 manual。
   func TestMirror_MarkManual(t *testing.T) {
	   machine, _ := newTestMirrorMachine()
	   ctx := context.Background()
	  hopID := uuid.NewString()
	  _, _ = machine.Create(ctx, hopID, "user-e", "", "ETH", "funding", "margin",
		  decimal.NewFromInt(2))
	   mv, err := machine.MarkManual(ctx, hopID, "ops-admin", "manual review required")
	   if err != nil {
		  t.Fatalf("MarkManual: %v", err)
	  }
	  if mv.State != xfermirror.MirrorManual {
		   t.Errorf("expected manual, got %s", mv.State)
	  }
	   if mv.OverrideActor != "ops-admin" {
		   t.Errorf("override_actor mismatch")
	  }
   }

// TestMirror_MarkManual_EmptyActorReject 空 actor 应被拒绝。
   func TestMirror_MarkManual_EmptyActorReject(t *testing.T) {
	   machine, _ := newTestMirrorMachine()
	   ctx := context.Background()
	   hopID := uuid.NewString()
	   _, _ = machine.Create(ctx, hopID, "user-f", "", "USDT", "funding", "trading",
		   decimal.NewFromInt(10))
	   _, err := machine.MarkManual(ctx, hopID, "", "reason")
	  if err == nil {
		   t.Error("expected error for empty actor, got nil")
	  }
  }

// TestMirror_CreditedThenRollback 已 credited 后不能 rollback。
   func TestMirror_CreditedThenRollback(t *testing.T) {
	  machine, _ := newTestMirrorMachine()
	  ctx := context.Background()
	  hopID := uuid.NewString()
	  _, _ = machine.Create(ctx, hopID, "user-g", "user-h", "USDT", "funding", "earn",
		   decimal.NewFromInt(100))
	   _, _ = machine.MarkCredited(ctx, hopID, "e1", "p1")
	   _, err := machine.MarkRolledBack(ctx, hopID, "e2", "p2")
	  if err == nil {
		  t.Error("expected error when rolling back already-credited mirror")
	   }
  }

// TestMirror_RolledBackThenCredit 已 rolled_back 后不能 credit。
   func TestMirror_RolledBackThenCredit(t *testing.T) {
	  machine, _ := newTestMirrorMachine()
	  ctx := context.Background()
	  hopID := uuid.NewString()
	   _, _ = machine.Create(ctx, hopID, "user-i", "user-j", "BTC", "funding", "trading",
		   decimal.NewFromFloat(0.1))
	   _, _ = machine.MarkRolledBack(ctx, hopID, "e3", "p3")
	  _, err := machine.MarkCredited(ctx, hopID, "e4", "p4")
	  if err == nil {
		   t.Error("expected error when crediting already-rolled-back mirror")
	  }
   }

// TestMirror_Align_Rollback 回滚收口后，initiator=rollback 与 mirror=rolled_back 一致。
   func TestMirror_Align_Rollback(t *testing.T) {
	   machine, _ := newTestMirrorMachine()
	  ctx := context.Background()
	   hopID := uuid.NewString()
	  _, _ = machine.Create(ctx, hopID, "user-k", "", "USDT", "funding", "trading",
		   decimal.NewFromInt(200))
	  _, _ = machine.MarkRolledBack(ctx, hopID, "rb-e", "plan-rb")
	   result, err := machine.Align(ctx, hopID, "rollback")
	  if err != nil {
		   t.Fatalf("Align: %v", err)
	  }
	  if !result.Aligned {
		   t.Errorf("expected aligned, conflict: %s", result.Conflict)
	  }
   }

// TestMirror_Align_Conflict 回滚收口但镜像为 credited（冲突）。

  func TestMirror_Align_Conflict(t *testing.T) {
	  machine,
		  _ := newTestMirrorMachine()
	  ctx := context.Background()
	  hopID := uuid.NewString()
	   _, _ = machine.Create(ctx, hopID, "user-l", "", "USDT", "funding", "earn",
		   decimal.NewFromInt(100))
	  _, _ = machine.MarkCredited(ctx, hopID, "e-c", "p-c")
	  result, err := machine.Align(ctx, hopID, "rollback")
	   if err != nil {
		  t.Fatalf("Align: %v", err)
	  }
	   if result.Aligned {
		  t.Error("expected conflict, got aligned")
	   }
	   if result.Conflict == "" {
		  t.Error("expected conflict message, got empty")
	   }
  }

// TestMirror_Events 镜像状态变更审计事件按时间记录。

  func TestMirror_Events(t *testing.T) {
	   machine, _ := newTestMirrorMachine()
	   ctx := context.Background()
	  hopID := uuid.NewString()
	   _, _ = machine.Create(ctx, hopID, "user-m", "user-n", "USDT", "funding", "trading",
		   decimal.NewFromInt(100))
	  _, _ = machine.MarkCredited(ctx, hopID, "e-ev", "plan-ev")
	  events, err := machine.GetEvents(ctx, hopID)
	  if err != nil {
		   t.Fatalf("GetEvents: %v", err)
	  }
	  if len(events) == 0 {
		  t.Error("expected at least 1 event")
	   }
	  lastEvt := events[len(events)-1]
	  if lastEvt.ToState != xfermirror.MirrorCredited {
		   t.Errorf("last event to_state: expected credited, got %s", lastEvt.ToState)
	   }
   }

// TestMirror_GetOrCreate_Idempotent 重复 GetOrCreate 返回同一记录。

   func TestMirror_GetOrCreate_Idempotent(t *testing.T) {
	   machine, _ := newTestMirrorMachine()
	  ctx := context.Background()
	  hopID := uuid.NewString()
	   mv1, err1 := machine.GetOrCreate(ctx, hopID, "user-o", "", "USDT", "funding", "trading",
		   decimal.NewFromInt(50))
	   if err1 != nil {
		  t.Fatalf("first GetOrCreate: %v", err1)
	   }
	   mv2, err2 := machine.GetOrCreate(ctx, hopID, "user-o", "", "USDT", "funding", "trading",
		   decimal.NewFromInt(50))
	   if err2 != nil {
		   t.Fatalf("second GetOrCreate: %v", err2)
	  }
	   if mv1.ID != mv2.ID {
		  t.Error("expected same ID on idempotent GetOrCreate")
	   }
  }
