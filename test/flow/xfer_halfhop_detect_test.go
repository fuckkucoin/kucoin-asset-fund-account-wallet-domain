package flow

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/xferclose"
	"account-assets.kucoin.net/funding/internal/xferstore"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// stubXferSource 存根 XferSource，空的 xferstore 返回。

type stubXferSource struct{}

  func (s *stubXferSource) ListOpen(_ context.Context, offset, limit int) ([]xferstore.Rec, error) {
	   return nil, nil
  }

  func (s *stubXferSource) Get(_ context.Context, idem string) (xferstore.Rec, error) {
	  return xferstore.Rec{}, nil
  }

   func newTestDetector(t *testing.T) (*xferclose.Detector, *xferclose.MemStore, *xferclose.SLABoard) {
	  t.Helper()
	   store := xferclose.NewMemStore()
	  sla := xferclose.NewSLABoard()
	   sla.Replace([]xferclose.XferSLA{
		   {
			  ID:   "sla-test",
			  Name: "test-sla",
			  Timeout: 100 * time.Millisecond,
			  EscalateAfter: 0,
			   AutoCloseout:  true,
			  DefaultAction: xferclose.ActionRollbackEgress,
			  Enabled:       true,
			   Priority:      1,
		   },
	   })
	  src := &stubXferSource{}
	   detector := xferclose.NewDetector(store, src, sla)
	  detector.WithMinAge(0)
	   return detector, store, sla
   }

// TestDetector_InjectHalfHop 注入半截工单，验证工单被创建且字段正确。
   func TestDetector_InjectHalfHop(t *testing.T) {
	  detector, store, _ := newTestDetector(t)
	  ctx := context.Background()
	   hopID := uuid.NewString()
	  hop, err := detector.InjectHalfHop(ctx, hopID, "user-1", "USDT", "funding", "trading",
		   decimal.NewFromInt(100), xferclose.HopKindStranded)
	   if err != nil {
		   t.Fatalf("InjectHalfHop: %v", err)
	   }
	   if hop.State != xferclose.HopDetected {
		   t.Errorf("expected state detected, got %s", hop.State)
	  }
	   if hop.HopID != hopID {
		  t.Errorf("hop_id mismatch: want %s, got %s", hopID, hop.HopID)
	   }
	   if hop.Asset != "USDT" {
		  t.Errorf("asset mismatch: want USDT, got %s", hop.Asset)
	   }
	  got, err := store.GetByHopID(ctx, hopID)
	  if err != nil {
		   t.Fatalf("GetByHopID: %v", err)
	   }
	  if got.ID != hop.ID {
		  t.Errorf("stored hop ID mismatch")
	  }
	   if got.Kind != xferclose.HopKindStranded {
		   t.Errorf("kind mismatch: want stranded, got %s", got.Kind)
	  }
   }

// TestDetector_SLABreachAfterRefresh 注入工单后等待 SLA 超限，验证 RefreshSLABreached 标记。
   func TestDetector_SLABreachAfterRefresh(t *testing.T) {
	  detector, store, _ := newTestDetector(t)
	   ctx := context.Background()
	  hopID := uuid.NewString()
	   _, err := detector.InjectHalfHop(ctx, hopID, "user-2", "BTC", "funding", "earn",
		  decimal.NewFromFloat(0.01), xferclose.HopKindTimeout)
	   if err != nil {
		   t.Fatalf("InjectHalfHop: %v", err)
	   }
	  // 等待 SLA 超限（100ms + buffer）。
	   time.Sleep(200 * time.Millisecond)
	   count, err := detector.RefreshSLABreached(ctx)
	   if err != nil {
		  t.Fatalf("RefreshSLABreached: %v", err)
	   }
	  if count == 0 {
		   t.Error("expected at least 1 hop to be marked breached")
	  }
	  hop, _ := store.GetByHopID(ctx, hopID)
	  if !hop.SLABreached {
		   t.Errorf("expected SLABreached=true after refresh")
	  }
   }

// TestDetector_DuplicateInjectIdempotent 同 hop_id 重复注入返回 ErrConflict，不创建第二条。

   func TestDetector_DuplicateInjectIdempotent(t *testing.T) {
	  detector, store, _ := newTestDetector(t)
	  ctx := context.Background()
	   hopID := uuid.NewString()
	   _, err := detector.InjectHalfHop(ctx, hopID, "user-3", "USDT", "funding", "trading",
		  decimal.NewFromInt(50), xferclose.HopKindOpen)
	   if err != nil {
		  t.Fatalf("first inject: %v", err)
	   }
	   _, err2 := detector.InjectHalfHop(ctx, hopID, "user-3", "USDT", "funding", "trading",
		  decimal.NewFromInt(50), xferclose.HopKindOpen)
	  if err2 == nil {
		   t.Error("expected error on duplicate inject, got nil")
	  }
	  all, _ := store.ListActive(ctx, 0, 100)
	  count := 0
	  for _, h := range all {
		   if h.HopID == hopID {
			   count++
		  }
	  }
	  if count != 1 {
		  t.Errorf("expected exactly 1 hop for hop_id, got %d", count)
	  }
   }

// TestDetector_ZeroAmountReject 零金额被拒绝。

  func TestDetector_ZeroAmountReject(t *testing.T) {
	  detector, _, _ := newTestDetector(t)
	  ctx := context.Background()
	   _, err := detector.InjectHalfHop(ctx, uuid.NewString(), "user-4", "USDT", "funding", "trading",
		   decimal.Zero, xferclose.HopKindOpen)
	   if err == nil {
		  t.Error("expected error for zero amount, got nil")
	   }
  }

// TestDetector_EmptyHopIDReject 空 hop_id 被拒绝。

   func TestDetector_EmptyHopIDReject(t *testing.T) {
	  detector, _, _ := newTestDetector(t)
	   ctx := context.Background()
	  _, err := detector.InjectHalfHop(ctx, "", "user-5", "USDT", "funding", "trading",
		  decimal.NewFromInt(10), xferclose.HopKindOpen)
	  if err == nil {
		  t.Error("expected error for empty hop_id, got nil")
	   }
   }

// TestDetector_ListActive_Pagination 分页列出活跃工单。

  func TestDetector_ListActive_Pagination(t *testing.T) {
	  detector,
		   store, _ := newTestDetector(t)
	   ctx := context.Background()
	  for i := 0; i < 7; i++ {
		  _, _ = detector.InjectHalfHop(ctx, uuid.NewString(), "user-6", "USDT",
			   "funding", "trading", decimal.NewFromInt(int64(i+1)), xferclose.HopKindOpen)
	  }
	  page1, _ := store.ListActive(ctx, 0, 5)
	  if len(page1) != 5 {
		   t.Errorf("page1: expected 5, got %d", len(page1))
	  }
	   page2, _ := store.ListActive(ctx, 5, 5)
	  if len(page2) != 2 {
		  t.Errorf("page2: expected 2, got %d", len(page2))
	  }
  }

// TestDetector_SLABoardMatch 验证 SLA 规则按先级匹配。
   func TestDetector_SLABoardMatch(t *testing.T) {
	   sla := xferclose.NewSLABoard()
	   sla.Replace([]xferclose.XferSLA{
		   {
			   ID:     "sla-btc",
			   Asset:  "BTC",
			  AmtMin: decimal.Zero,
			   Timeout: 5 * time.Minute,
			  Enabled: true,
			   Priority: 10,
		   },
		   {
			  ID:       "sla-any",
			   Timeout:  30 * time.Minute,
			   Enabled:  true,
			   Priority: 100,
		   },
	  })
	  m, ok := sla.Match("BTC", "funding", "trading", decimal.NewFromInt(1), time.Now())
	  if !ok {
		   t.Fatal("expected match for BTC")
	   }
	   if m.Rule.ID != "sla-btc" {
		  t.Errorf("expected sla-btc, got %s", m.Rule.ID)
	   }
	   m2, ok2 := sla.Match("ETH", "funding", "trading", decimal.NewFromInt(1), time.Now())
	   if !ok2 {
		   t.Fatal("expected match for ETH (fallback)")
	   }
	   if m2.Rule.ID != "sla-any" {
		   t.Errorf("expected sla-any, got %s", m2.Rule.ID)
	  }
  }
