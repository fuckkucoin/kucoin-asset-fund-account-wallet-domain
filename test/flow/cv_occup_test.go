package flow

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/cvoccup"
	"account-assets.kucoin.net/funding/internal/cvrollback"
	"github.com/shopspring/decimal"
)

// --- 辅助 ---
// stubHoldAPI 用 hold 存根。
type stubHoldAPI struct {
	holds map[string]decimal.Decimal // holdID → amount
	counter int
}

  func newStubHoldAPI() *stubHoldAPI {
	  return &stubHoldAPI{holds: map[string]decimal.Decimal{}}
   }

  func (s *stubHoldAPI) PlaceConvertHold(_ context.Context, _, _, _, amount, ref string) (string, error) {
	   s.counter++
	   id := "hold-" + ref
	  amt, _ := decimal.NewFromString(amount)
	  s.holds[id] = amt
	   return id, nil
   }

   func (s *stubHoldAPI) ReleaseHold(_ context.Context, _, holdID string) error {
	   delete(s.holds, holdID)
	  return nil
  }

  func (s *stubHoldAPI) GetHoldAmount(_ context.Context, _, holdID string) (decimal.Decimal, error) {
	  if amt, ok := s.holds[holdID]; ok {
		   return amt, nil
	   }
	   return decimal.NewFromInt(-1), nil
  }

// stubAvailReader 用可用余额存根。

type stubAvailReader struct {
	avail map[string]decimal.Decimal
}

  func newStubAvailReader(pairs ...string) *stubAvailReader {
	  s := &stubAvailReader{avail: map[string]decimal.Decimal{}}
	   for i := 0; i+1 < len(pairs); i += 2 {
		   s.avail[pairs[i]],
			  _ = decimal.NewFromString(pairs[i+1])
	  }
	   return s
   }

  func (s *stubAvailReader) AvailBalance(_ context.Context, userID, asset, _ string) (decimal.Decimal, error) {
	  key := userID + ":" + asset
	  if v, ok := s.avail[key]; ok {
		  return v, nil
	   }
	  return decimal.NewFromString("10000")
  }

// buildOccSvc 构造用占用服务。
  func buildOccSvc(hold *stubHoldAPI, avail *stubAvailReader) (*cvoccup.Service, *cvoccup.MemStore) {
	   store := cvoccup.NewMemStore()
	  svc := cvoccup.NewService(store, hold, avail)
	  return svc, store
   }

// --- 用例 ---
// TestOccupyAndSettle 限价路径开立占用后全量成交结算：占用令牌清零。
  func TestOccupyAndSettle(t *testing.T) {
	  hold := newStubHoldAPI()
	  avail := newStubAvailReader()
	  svc, _ := buildOccSvc(hold, avail)
	  ctx := context.Background()
	   res, err := svc.Occupy(ctx, cvoccup.OccupyReq{
		  UserID:  "u1",
		  OrderID: "ord-1",
		   ClientOID: "coid-1",
		  PathKind:  cvoccup.PathLimit,
		   FromAsset: "USDT",
		   ToAsset:   "BTC",
		  TotalAmount: "1000",
		   AccountType: "MAIN",
		  ExpireAt:    time.Now().Add(time.Hour),
	  })
	  if err != nil {
		   t.Fatalf("Occupy failed: %v", err)
	   }
	   if res.Occ.Status != cvoccup.OccFrozen {
		  t.Fatalf("expected FROZEN, got %s", res.Occ.Status)
	  }
	   if res.HoldID == "" {
		   t.Fatal("expected hold token")
	   }
	  // 全量成交：先推进部分（满足全量），再结算
	   occ, err := svc.PartialAdvance(ctx, res.Occ.ID, "1000", "fill-1")
	  if err != nil {
		   t.Fatalf("PartialAdvance failed: %v", err)
	   }
	  if occ.RemainAmount != "0" {
		  t.Fatalf("expected remain=0, got %s", occ.RemainAmount)
	  }
	   settled,
		  err := svc.Settle(ctx, res.Occ.ID)
	   if err != nil {
		   t.Fatalf("Settle failed: %v", err)
	   }
	   if settled.Status != cvoccup.OccSettled {
		  t.Fatalf("expected SETTLED, got %s", settled.Status)
	   }
	  if settled.HoldToken != "" {
		   t.Fatal("expected hold token cleared after settle")
	  }
	  // holdcat 中 hold 应已释放
	   if len(hold.holds) != 0 {
		  t.Fatalf("hold not released: %v", hold.holds)
	   }
  }

// TestPartialFillThenCancelRollback PARTIAL 后取消剩余：回滚腿释放剩余占用，账本双边平衡。

  func TestPartialFillThenCancelRollback(t *testing.T) {
	   hold := newStubHoldAPI()
	   avail := newStubAvailReader()
	  svc, _ := buildOccSvc(hold, avail)
	   rbStore := cvrollback.NewMemStore()
	   orch := cvrollback.NewOrchestrator(rbStore,
		   &testOccReleaser{svc: svc},
		  nil, nil)
	  ctx := context.Background()
	  res, err := svc.Occupy(ctx, cvoccup.OccupyReq{
		  UserID:  "u2",
		   OrderID: "ord-2",
		   ClientOID: "coid-2",
		  PathKind:  cvoccup.PathLimit,
		  FromAsset: "USDT",
		   ToAsset:     "ETH",
		   TotalAmount: "500",
		   AccountType: "MAIN",
		  ExpireAt:    time.Now().Add(time.Hour),
	  })
	  if err != nil {
		  t.Fatalf("Occupy failed: %v", err)
	   }
	  // 部分成交 200
	  _, err = svc.PartialAdvance(ctx, res.Occ.ID, "200", "fill-1")
	   if err != nil {
		   t.Fatalf("PartialAdvance failed: %v", err)
	  }
	   // 取消剩余 300：触发回滚
	   plan, err := orch.PlanAndExecute(ctx, cvrollback.PlanReq{
		  OccID:   res.Occ.ID,
		   OrderID: "ord-2",
		  UserID: "u2",
		  Trigger:      cvrollback.TriggerCancel,
		   FilledAmount: "200",
		  RemainAmount: "300",
	   })
	   if err != nil {
		   t.Fatalf("PlanAndExecute failed: %v", err)
	   }
	  if plan.Status != cvrollback.RollbackDone {
		  t.Fatalf("expected DONE, got %s: %s", plan.Status, plan.Error)
	  }
	   // 验证占用已释放
	  occ, _ := svc.GetOcc(ctx, res.Occ.ID)
	  if occ.Status != cvoccup.OccRolledBack {
		   t.Fatalf("expected ROLLED_BACK, got %s", occ.Status)
	   }
	  // holdcat 中 hold 已释放
	  if len(hold.holds) != 0 {
		   t.Fatalf("hold not released after rollback: %v", hold.holds)
	   }
   }

// TestIntegrityGateBlocksTokenDrift Integrity Gate 阻断占用令牌漂移。

   func TestIntegrityGateBlocksTokenDrift(t *testing.T) {
	   hold := newStubHoldAPI()
	  avail := newStubAvailReader()
	   svc, store := buildOccSvc(hold, avail)
	   ctx := context.Background()
	   res, err := svc.Occupy(ctx, cvoccup.OccupyReq{
		  UserID: "u3",
		   OrderID: "ord-3",
		   ClientOID: "coid-3",
		   PathKind:  cvoccup.PathLimit,
		   FromAsset: "USDT",
		   ToAsset:   "BTC",
		   TotalAmount: "1000",
		   AccountType: "MAIN",
		   ExpireAt: time.Now().Add(time.Hour),
	  })
	  if err != nil {
		   t.Fatalf("Occupy failed: %v", err)
	  }
	   // 人为破坏：删除 holdcat 中的 hold（漂移）
	  delete(hold.holds, res.HoldID)
	  // CheckIntegrity 应能检出漂移（hold 消失但 occ 仍 FROZEN）
	  err = svc.CheckIntegrity(ctx, res.Occ.ID, true)
	  if err == nil {
		   t.Fatal("expected integrity error after hold removed")
	   }
	  // 验证 Breach 已开
	  breaches, _ := svc.ListBreaches(ctx, true, 10)
	  if len(breaches) == 0 {
		  t.Fatal("expected breach to be opened on integrity failure")
	  }
	  // 验证 store 中 occ 有 breach_id（通过直接访问 store）
	   occ, _ := store.GetOcc(ctx, res.Occ.ID)
	   if occ.BreachID == "" {
		   t.Fatal("expected breach_id set on occ")
	  }
  }

// TestConcurrentPathsSameAsset 并发两路径同资产：占用汇总不超过可用余额。

   func TestConcurrentPathsSameAsset(t *testing.T) {
	  hold := newStubHoldAPI()
	  // 可用余额 1500 USDT
	  avail := newStubAvailReader("u4:USDT", "1500")
	  svc, _ := buildOccSvc(hold, avail)
	   ctx := context.Background()
	  // 第一条路径占用 800
	   res1, err := svc.Occupy(ctx, cvoccup.OccupyReq{
		   UserID: "u4",
		  OrderID:   "ord-4a",
		  ClientOID: "coid-4a",
		   PathKind:    cvoccup.PathLimit,
		  FromAsset:   "USDT",
		  ToAsset:     "BTC",
		  TotalAmount: "800",
		   AccountType: "MAIN",
		  ExpireAt: time.Now().Add(time.Hour),
	   })
	  if err != nil {
		  t.Fatalf("Occupy path1 failed: %v", err)
	  }
	   if res1.Occ.Status != cvoccup.OccFrozen {
		  t.Fatalf("path1 not FROZEN: %s", res1.Occ.Status)
	   }
	  // 第二条路径占用 600（800+600=1400 ≤ 1500，合法）
	  res2, err := svc.Occupy(ctx, cvoccup.OccupyReq{
		  UserID: "u4",
		   OrderID:   "ord-4b",
		   ClientOID: "coid-4b",
		   PathKind:  cvoccup.PathLimit,
		   FromAsset: "USDT",
		  ToAsset: "ETH",
		   TotalAmount: "600",
		  AccountType: "MAIN",
		  ExpireAt: time.Now().Add(time.Hour),
	  })
	  if err != nil {
		   t.Fatalf("Occupy path2 failed: %v", err)
	  }
	   if res2.Occ.Status != cvoccup.OccFrozen {
		   t.Fatalf("path2 not FROZEN: %s", res2.Occ.Status)
	  }
	  // 第三条路径试图再占 300（800+600+300=1700 > 1500，应被拒绝）
	  _, err = svc.Occupy(ctx, cvoccup.OccupyReq{
		   UserID:      "u4",
		   OrderID:     "ord-4c",
		   ClientOID:   "coid-4c",
		   PathKind:    cvoccup.PathLimit,
		   FromAsset:   "USDT",
		   ToAsset:     "BNB",
		   TotalAmount: "300",
		  AccountType: "MAIN",
		   ExpireAt: time.Now().Add(time.Hour),
	   })
	   if err == nil {
		   t.Fatal("expected Occupy to be rejected when total occ exceeds available")
	  }
   }

// TestExpireRollbackWorker 过期路径触发自动回滚。
   func TestExpireRollbackWorker(t *testing.T) {
	   hold := newStubHoldAPI()
	   avail := newStubAvailReader()
	   svc, _ := buildOccSvc(hold, avail)
	  ctx := context.Background()
	   // 创建已过期路径
	  res, err := svc.Occupy(ctx, cvoccup.OccupyReq{
		   UserID:  "u5",
		  OrderID: "ord-5",
		   ClientOID: "coid-5",
		  PathKind:  cvoccup.PathLimit,
		   FromAsset: "USDT",
		   ToAsset: "BTC",
		  TotalAmount: "100",
		  AccountType: "MAIN",
		  ExpireAt:    time.Now().Add(-time.Second), // 已过期
	   })
	  if err != nil {
		  t.Fatalf("Occupy failed: %v", err)
	   }
	  worker := cvoccup.NewExpireRollbackWorker(svc, nil, time.Minute)
	   n, err := worker.RunOnce(ctx)
	   if err != nil {
		   t.Fatalf("RunOnce failed: %v", err)
	  }
	  if n != 1 {
		   t.Fatalf("expected 1 expired rollback, got %d", n)
	   }
	   occ, _ := svc.GetOcc(ctx, res.Occ.ID)
	  if occ.Status != cvoccup.OccRolledBack {
		   t.Fatalf("expected ROLLED_BACK after expire worker, got %s", occ.Status)
	  }
   }

// TestBreachBlockNewPath 穿仓时阻断新路径开立。
   func TestBreachBlockNewPath(t *testing.T) {
	   hold := newStubHoldAPI()
	   // 可用余额很少 50 USDT
	   avail := newStubAvailReader("u6:USDT", "50")
	   svc, _ := buildOccSvc(hold, avail)
	  ctx := context.Background()
	  // 占用 200 > 50：应被拒绝并开 Breach
	   _, err := svc.Occupy(ctx, cvoccup.OccupyReq{
		   UserID: "u6",
		  OrderID:   "ord-6",
		  ClientOID: "coid-6",
		   PathKind:  cvoccup.PathLimit,
		  FromAsset: "USDT",
		  ToAsset: "BTC",
		  TotalAmount: "200",
		  AccountType: "MAIN",
		  ExpireAt:    time.Now().Add(time.Hour),
	   })
	   if err == nil {
		  t.Fatal("expected Occupy rejected for negative available")
	  }
	  breaches, _ := svc.ListBreaches(ctx, true, 10)
	  if len(breaches) == 0 {
		  t.Fatal("expected breach opened when available goes negative")
	   }
	  if breaches[0].BreachKind != cvoccup.BreachAvailNeg {
		   t.Fatalf("expected breach kind %s, got %s", cvoccup.BreachAvailNeg, breaches[0].BreachKind)
	  }
  }

// TestSettleFailAfterFreeze 冻后结算失败：Integrity Gate 阻断并开 Breach。

   func TestSettleFailAfterFreeze(t *testing.T) {
	   hold := newStubHoldAPI()
	   avail := newStubAvailReader()
	  svc, _ := buildOccSvc(hold, avail)
	   ctx := context.Background()
	   res, err := svc.Occupy(ctx, cvoccup.OccupyReq{
		  UserID:  "u7",
		  OrderID: "ord-7",
		   ClientOID: "coid-7",
		   PathKind:  cvoccup.PathLimit,
		   FromAsset: "USDT",
		   ToAsset: "BTC",
		  TotalAmount: "500",
		  AccountType: "MAIN",
		  ExpireAt: time.Now().Add(time.Hour),
	   })
	   if err != nil {
		   t.Fatalf("Occupy failed: %v", err)
	  }
	   // 人为使 hold 量与 remain 不一致（漂移）
	   hold.holds[res.HoldID] = decimal.NewFromInt(300) // 应为 500，人为改为 300
	  // CheckIntegrity 应检出漂移
	   err = svc.CheckIntegrity(ctx, res.Occ.ID, true)
	   if err == nil {
		  t.Fatal("expected integrity error for hold/remain mismatch")
	   }
	  breaches, _ := svc.ListBreaches(ctx, true, 10)
	   if len(breaches) == 0 {
		  t.Fatal("expected breach for hold/remain mismatch")
	   }
   }

// testOccReleaser 用 OccReleaser 适配器。

type testOccReleaser struct {
	svc *cvoccup.Service
}

   func (r *testOccReleaser) Release(ctx context.Context, occID, reason string) error {
	   _, err := r.svc.Release(ctx, occID, reason)
	  return err
  }

  func (r *testOccReleaser) CheckIntegrity(ctx context.Context, occID string, openBreach bool) error {
	   return r.svc.CheckIntegrity(ctx, occID, openBreach)
  }
