package flow_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/holdpatrol"
	"account-assets.kucoin.net/funding/internal/holdrel"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/rs/zerolog"
)

// fixedLedger 返回固定冻结量的 LedgerFreezeReader（用）。

  func fixedLedger(frozen string) holdpatrol.LedgerFreezeReader {
	   return func(_ context.Context, _, _, _ string) (string, error) {
		   return frozen, nil
	  }
   }

// mapLedger 按 userID+asset 返回不同冻结量。
   func mapLedger(m map[string]string) holdpatrol.LedgerFreezeReader {
	  return func(_ context.Context, userID, asset, _ string) (string, error) {
		  k := userID + ":" + asset
		   if v, ok := m[k]; ok {
			   return v, nil
		   }
		   return "0", nil
	  }
   }

   func newPatrolStack(cfg holdpatrol.Config, ledger holdpatrol.LedgerFreezeReader) (
	   *holdcat.TokenBook, *holdrel.GapStore, *holdpatrol.Patrol,
   ) {
	  kv := persist.NewMemKV()
	   tokens := holdcat.NewTokenBook(kv)
	  gaps := holdrel.NewGapStore(kv)
	  patrol := holdpatrol.NewPatrol(tokens, gaps, ledger, cfg, zerolog.Nop())
	   return tokens, gaps, patrol
  }

// TestPatrol_OrphanTag_LedgerZero 账本已无冻结但 token active → 开孤儿缺口单。

   func TestPatrol_OrphanTag_LedgerZero(t *testing.T) {
	   cfg := holdpatrol.DefaultConfig()
	   cfg.ScanLimit = 100
	  tokens, gaps, patrol := newPatrolStack(cfg, fixedLedger("0"))
	   tok, err := tokens.Place(context.Background(), "u1", "USDT", "funding", "500",
		  holdcat.LayerRisk, holdcat.ReasonKYTHighRisk, "", "idem-patrol-orphan", "", "")
	   if err != nil {
		   t.Fatalf("place: %v", err)
	  }
	   run := patrol.Run(context.Background())
	   if run.Err != "" {
		   t.Fatalf("run error: %s", run.Err)
	  }
	  if run.GapsOpened == 0 {
		   t.Error("want ≥1 gap opened for orphan tag, got 0")
	  }
	  gapList, err := gaps.List(context.Background(), "u1", holdrel.GapStateOpen, 10)
	  if err != nil {
		  t.Fatalf("list gaps: %v", err)
	  }
	   if len(gapList) == 0 {
		  t.Error("want at least 1 open gap")
	  }
	  found := false
	  for _, g := range gapList {
		  if g.TokenID == tok.ID && g.Kind == holdrel.GapKindOrphanTag {
			  found = true
		   }
	  }
	   if !found {
		  t.Errorf("gap with kind=orphan_tag not found for token %s", tok.ID)
	   }
   }

// TestPatrol_AmountDrift token 金额 > 账本冻结量 → 开漂移缺口单。

  func TestPatrol_AmountDrift(t *testing.T) {
	   cfg := holdpatrol.DefaultConfig()
	  // 账本冻结量 200，但 token 金额 500（漂移 300）
	   tokens, gaps, patrol := newPatrolStack(cfg, fixedLedger("200"))
	  tok, _ := tokens.Place(context.Background(), "u1", "USDT", "funding", "500",
		   holdcat.LayerAML, holdcat.ReasonSuspiciousTx, "", "idem-patrol-drift", "", "")
	   run := patrol.Run(context.Background())
	  if run.GapsOpened == 0 {
		  t.Error("want ≥1 gap for amount drift")
	   }
	   gapList, _ := gaps.List(context.Background(), "u1", holdrel.GapStateOpen, 10)
	  found := false
	  for _, g := range gapList {
		  if g.TokenID == tok.ID && g.Kind == holdrel.GapKindAmountDrift {
			  found = true
		  }
	  }
	  if !found {
		   t.Errorf("amount_drift gap not found for token %s", tok.ID)
	   }
  }

// TestPatrol_NoDrift_NoGap 正常冻结（token 金额 ≤ 账本冻结量）→ 不开缺口单。

   func TestPatrol_NoDrift_NoGap(t *testing.T) {
	   cfg := holdpatrol.DefaultConfig()
	   // 账本冻结量 1000，token 金额 500（无漂移）
	  tokens, gaps, patrol := newPatrolStack(cfg, fixedLedger("1000"))
	   _, _ = tokens.Place(context.Background(), "u1", "USDT", "funding", "500",
		  holdcat.LayerOps, holdcat.ReasonOpsManualHold, "", "idem-patrol-nodrift", "", "")
	   run := patrol.Run(context.Background())
	   if run.Err != "" {
		  t.Fatalf("run error: %s", run.Err)
	   }
	   if run.GapsOpened != 0 {
		  t.Errorf("want 0 gaps for no drift, got %d", run.GapsOpened)
	   }
	   gapList, _ := gaps.List(context.Background(), "u1", holdrel.GapStateOpen, 10)
	   if len(gapList) != 0 {
		   t.Errorf("want 0 open gaps, got %d", len(gapList))
	  }
   }

// TestPatrol_ReleasingTimeout releasing 超时 → 开超时缺口单。

   func TestPatrol_ReleasingTimeout(t *testing.T) {
	   cfg := holdpatrol.Config{
		   ReleasingTimeout: 1 * time.Millisecond, // 极短超时供
		   ActiveMaxAge:          72 * time.Hour,
		  ScanLimit:             100,
		   AutoReleaseOrphanTags: false,
	   }
	  tokens, gaps, patrol := newPatrolStack(cfg, fixedLedger("1000"))
	  tok, _ := tokens.Place(context.Background(), "u1", "USDT", "funding", "500",
		   holdcat.LayerRisk, holdcat.ReasonManualRiskHold, "", "idem-patrol-timeout", "", "")
	   _, _ = tokens.BeginRelease(context.Background(), tok.ID) // → releasing
	  // 等超过 1ms
	   time.Sleep(5 * time.Millisecond)
	  run := patrol.Run(context.Background())
	  if run.GapsOpened == 0 {
		   t.Error("want ≥1 gap for releasing timeout")
	   }
	  gapList, _ := gaps.List(context.Background(), "u1", holdrel.GapStateOpen, 10)
	  found := false
	   for _, g := range gapList {
		  if g.TokenID == tok.ID && g.Kind == holdrel.GapKindTimeout {
			   found = true
		   }
	  }
	  if !found {
		   t.Errorf("timeout_escalation gap not found")
	  }
   }

// TestPatrol_ActiveMaxAge active 超龄 → 开超时缺口单。

   func TestPatrol_ActiveMaxAge(t *testing.T) {
	   cfg := holdpatrol.Config{
		  ReleasingTimeout: 15 * time.Minute,
		   ActiveMaxAge:          1 * time.Millisecond, // 极短
		  ScanLimit:             100,
		   AutoReleaseOrphanTags: false,
	  }
	   tokens, gaps, patrol := newPatrolStack(cfg, fixedLedger("1000"))
	   tok, _ := tokens.Place(context.Background(), "u1", "USDT", "funding", "200",
		   holdcat.LayerOps, holdcat.ReasonPromoHold, "", "idem-patrol-maxage", "", "")
	  time.Sleep(5 * time.Millisecond)
	   run := patrol.Run(context.Background())
	   if run.GapsOpened == 0 {
		  t.Error("want ≥1 gap for active max age")
	   }
	   gapList, _ := gaps.List(context.Background(), "u1", holdrel.GapStateOpen, 10)
	   found := false
	   for _, g := range gapList {
		  if g.TokenID == tok.ID && g.Kind == holdrel.GapKindTimeout {
			   found = true
		   }
	   }
	   if !found {
		   t.Errorf("timeout_escalation gap not found for active max age")
	  }
  }

// TestPatrol_GapIdempotent 同 tokenID+kind 不重复开缺口单。

   func TestPatrol_GapIdempotent(t *testing.T) {
	   cfg := holdpatrol.DefaultConfig()
	   tokens, gaps, patrol := newPatrolStack(cfg, fixedLedger("0"))
	  _, _ = tokens.Place(context.Background(), "u1", "USDT", "funding", "100",
		  holdcat.LayerOps, holdcat.ReasonOpsManualHold, "", "idem-patrol-gap-idem", "", "")
	  // 运行两次
	  run1 := patrol.Run(context.Background())
	  run2 := patrol.Run(context.Background())
	   if run1.GapsOpened == 0 {
		   t.Error("first run should open gaps")
	   }
	  // 第二次因幂等不应开新单（同 tokenID+kind 已有 open 单）
	  _ = run2 // GapsOpened 可能为 0
	  gapList, _ := gaps.List(context.Background(), "u1", holdrel.GapStateOpen, 20)
	   if len(gapList) > run1.GapsOpened {
		  t.Errorf("idempotent: gaps count increased from %d to %d on second run",
			  run1.GapsOpened, len(gapList))
	   }
  }

// TestPatrol_GapStateMachine open → acked → closed。
   func TestPatrol_GapStateMachine(t *testing.T) {
	   ctx := context.Background()
	  kv := persist.NewMemKV()
	   gaps := holdrel.NewGapStore(kv)
	  gap, err := gaps.Open(ctx, holdrel.HoldGap{
		   TokenID: "tok-001", UserID: "u1", Asset: "USDT",
		  Kind: holdrel.GapKindAmountDrift, Detail: "test drift",
	   })
	  if err != nil {
		  t.Fatalf("open: %v", err)
	   }
	   if gap.State != holdrel.GapStateOpen {
		  t.Errorf("state=%v want open", gap.State)
	  }
	  // open → acked
	   gap2, err := gaps.Ack(ctx, gap.GapID, "ops-user")
	  if err != nil {
		  t.Fatalf("ack: %v", err)
	   }
	  if gap2.State != holdrel.GapStateAcked {
		   t.Errorf("state=%v want acked", gap2.State)
	   }
	  if gap2.AckedBy != "ops-user" {
		   t.Errorf("acked_by=%q", gap2.AckedBy)
	  }
	  // acked → closed
	   gap3, err := gaps.Close(ctx, gap.GapID, "ops-user", "resolved manually")
	  if err != nil {
		  t.Fatalf("close: %v", err)
	   }
	  if gap3.State != holdrel.GapStateClosed {
		   t.Errorf("state=%v want closed", gap3.State)
	   }
	   // closed 再次 close 幂等
	   gap4, err := gaps.Close(ctx, gap.GapID, "ops-user", "")
	  if err != nil {
		  t.Fatalf("close idempotent: %v", err)
	   }
	  if gap4.State != holdrel.GapStateClosed {
		  t.Error("should remain closed")
	  }
	  // closed 不可 ack
	   _, err = gaps.Ack(ctx, gap.GapID, "someone")
	   if !errors.Is(err, holdrel.ErrGapStateInvalid) {
		  t.Errorf("ack closed gap: want ErrGapStateInvalid, got %v", err)
	  }
  }

// TestPatrol_OpenCount 监控指标。
   func TestPatrol_OpenCount(t *testing.T) {
	  ctx := context.Background()
	   kv := persist.NewMemKV()
	  gaps := holdrel.NewGapStore(kv)
	  cnt0, _ := gaps.OpenCount(ctx)
	  if cnt0 != 0 {
		   t.Errorf("want 0 initially, got %d", cnt0)
	  }
	  for i := 0; i < 3; i++ {
		   _, _ = gaps.Open(ctx, holdrel.HoldGap{
			  TokenID: "tok-" + string(rune('A'+i)),
			   UserID: "u1", Asset: "USDT",
			   Kind: holdrel.GapKindAmountDrift, Detail: "test",
		  })
	   }
	   cnt3, _ := gaps.OpenCount(ctx)
	  if cnt3 != 3 {
		  t.Errorf("want 3 open gaps, got %d", cnt3)
	   }
	   // close 一个
	   list, _ := gaps.List(ctx, "u1", holdrel.GapStateOpen, 10)
	  if len(list) > 0 {
		   _, _ = gaps.Close(ctx, list[0].GapID, "ops", "")
	  }
	  cnt2, _ := gaps.OpenCount(ctx)
	  if cnt2 != 2 {
		  t.Errorf("want 2 after close, got %d", cnt2)
	   }
  }

// TestPatrol_MultiUserIsolation 多用户 token 互不干扰。
   func TestPatrol_MultiUserIsolation(t *testing.T) {
	  cfg := holdpatrol.DefaultConfig()
	   // u1 账本冻结量 0（孤儿），u2 账本正常（1000）
	  tokens, gaps, patrol := newPatrolStack(cfg, mapLedger(map[string]string{
		   "u1:USDT": "0",
		   "u2:USDT": "1000",
	  }))
	  _, _ = tokens.Place(context.Background(), "u1", "USDT", "funding", "500",
		  holdcat.LayerRisk, holdcat.ReasonVelocityBreach, "", "idem-iso-u1", "", "")
	   _, _ = tokens.Place(context.Background(), "u2", "USDT", "funding", "500",
		  holdcat.LayerOps, holdcat.ReasonOpsManualHold, "", "idem-iso-u2", "", "")
	  patrol.Run(context.Background())
	   // u1 应有缺口单
	   u1Gaps, _ := gaps.List(context.Background(), "u1", holdrel.GapStateOpen, 10)
	   if len(u1Gaps) == 0 {
		   t.Error("u1 should have orphan gap")
	  }
	   // u2 不应有缺口单
	   u2Gaps, _ := gaps.List(context.Background(), "u2", holdrel.GapStateOpen, 10)
	   if len(u2Gaps) != 0 {
		   t.Errorf("u2 should have no gaps, got %d", len(u2Gaps))
	  }
   }
