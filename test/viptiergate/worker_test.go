package viptiergate_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/viptiergate"
)

// testAlertHook 用于捕获告警事件（故障注入验证）。
type testAlertHook struct {
	events []viptiergate.AlertEvent
	count  atomic.Int32
}

  func (h *testAlertHook) Fire(_ context.Context, evt viptiergate.AlertEvent) error {
	  h.events = append(h.events, evt)
	  h.count.Add(1)
	  return nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// Worker Start/Stop 生命周期
// ─────────────────────────────────────────────────────────────────────────────

  func TestWorker_StartStop(t *testing.T) {
	   svc := viptiergate.NewService(nil)
	   w := viptiergate.NewWorker(svc, viptiergate.WorkerConfig{
		   FreezeScanInterval: 100 * time.Millisecond,
		   DriftScanInterval:     100 * time.Millisecond,
		  QuoteExpiryInterval:   100 * time.Millisecond,
		   LeaseProgressInterval: 100 * time.Millisecond,
	   }, nil)
	   ctx := context.Background()
	   w.Start(ctx)
	  // 幂等 Start（重复调用不 panic）。
	  w.Start(ctx)
	   time.Sleep(200 * time.Millisecond)
	  w.Stop()
   }

// ─────────────────────────────────────────────────────────────────────────────
// FreezeScanner：blocked 租约在冻额清零后推进至 switched
// ─────────────────────────────────────────────────────────────────────────────

  func TestFreezeScanner_UnblocksLease(t *testing.T) {
	   ctx := context.Background()
	   now := time.Now().UTC()
	  svc := viptiergate.NewService(nil)
	  svc.WithClock(fixedClock(now))
	  userID := "u-ws-scanner-001"
	   // 注册活跃冻额。
	  freeze, _ := svc.RegisterFreeze(ctx, userID, "BTC", "0.1", 5, viptiergate.TierVIP, 0)
	   // 降档时因冻额被阻塞——直接调用 GatedDowngrade 应返回 ErrInflightFreeze。
	  _, _, err := svc.GatedDowngrade(ctx, viptiergate.AcquireLeaseReq{
		   UserID: userID,
		  FromTier: viptiergate.TierVIP,
		  ToTier: viptiergate.TierStandard,
		  TriggerKind: "clear_pin",
		   TriggerRef:  userID + "|clear_pin|wscan1",
		  ExpectedGen: 0,
	   })
	   if err != viptiergate.ErrInflightFreeze {
		   t.Errorf("expected ErrInflightFreeze, got %v", err)
	  }
	  // 释放冻额。
	   if err := svc.ReleaseFreeze(ctx, freeze.ID); err != nil {
		   t.Fatalf("ReleaseFreeze: %v", err)
	   }
	  // 短间隔 Worker 扫描。
	   w := viptiergate.NewWorker(svc, viptiergate.WorkerConfig{
		   FreezeScanInterval: 10 * time.Millisecond,
		  DriftScanInterval: 10 * time.Millisecond,
		   QuoteExpiryInterval:   10 * time.Millisecond,
		  LeaseProgressInterval: 10 * time.Millisecond,
	  }, nil)
	  w.Start(ctx)
	   time.Sleep(100 * time.Millisecond)
	   w.Stop()
	   // 降档现在应该成功（冻额已释放，之前 blocked 的租约已由 worker 推进）。
	   _, newGen, err := svc.GatedDowngrade(ctx, viptiergate.AcquireLeaseReq{
		   UserID:   userID,
		  FromTier: viptiergate.TierVIP,
		  ToTier:   viptiergate.TierStandard,
		  TriggerKind: "clear_pin",
		   TriggerRef: userID + "|clear_pin|wscan2",
		   ExpectedGen: 0,
	  })
	  if err != nil {
		   t.Errorf("expected success after freeze release and worker scan, got %v", err)
	  }
	   if newGen != 1 {
		  t.Errorf("newGen: got %d, want 1", newGen)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// QuoteExpiryReaper：TTL 到期 QuoteBind 被清理
// ─────────────────────────────────────────────────────────────────────────────

  func TestQuoteExpiryReaper(t *testing.T) {
	   ctx := context.Background()
	  now := time.Now().UTC()
	   svc := viptiergate.NewService(nil)
	   svc.WithClock(fixedClock(now))
	  userID := "u-expiry-001"
	  // 颁发一个短 TTL QuoteBind（TTL = 1 秒）。
	  qb, err := svc.IssueQuote(ctx, viptiergate.IssueQuoteReq{
		  UserID: userID,
		  Asset: "SOL",
		  Gross: "10.0",
		   Fee:   "0.05",
		   Net:   "9.95",
		   BPS:   5,
		   Tier:  viptiergate.TierVIP,
		   TTL:   1,
	   })
	   if err != nil {
		  t.Fatalf("IssueQuote: %v", err)
	  }
	  // 初始状态：open。
	   got, _ := svc.GetQuoteBind(ctx, qb.ID)
	   if got.Status != viptiergate.QuoteBindOpen {
		   t.Errorf("initial status: got %s, want open", got.Status)
	   }
	  // 时钟推进到 TTL 之后。
	  svc.WithClock(fixedClock(now.Add(2 * time.Second)))
	   // 运行 worker 一次（短间隔）。
	  w := viptiergate.NewWorker(svc, viptiergate.WorkerConfig{
		  FreezeScanInterval:  5 * time.Millisecond,
		  DriftScanInterval:   5 * time.Millisecond,
		  QuoteExpiryInterval: 5 * time.Millisecond,
		   LeaseProgressInterval: 5 * time.Millisecond,
	  }, nil)
	  w.Start(ctx)
	   time.Sleep(50 * time.Millisecond)
	   w.Stop()
	  // QuoteBind 应被标记为 expired。
	   updated, _ := svc.GetQuoteBind(ctx, qb.ID)
	  if updated.Status != viptiergate.QuoteBindExpired {
		   t.Errorf("after worker run, status: got %s, want expired", updated.Status)
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// DriftPatrol 高危案件升级告警
// ─────────────────────────────────────────────────────────────────────────────

   func TestDriftPatrol_EscalateAlert(t *testing.T) {
	   ctx := context.Background()
	   now := time.Now().UTC()
	  svc := viptiergate.NewService(nil)
	  svc.WithClock(fixedClock(now))
	   userID := "u-escalate-001"
	   // 开立一个漂移案（将时间设为超出阈值的过去时间）。
	  svc.WithClock(fixedClock(now.Add(-15 * time.Minute))) // 15分钟前开案
	   _, err := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
		  UserID: userID,
		   Asset:  "BTC",
		   Kind: viptiergate.DriftBPSStale,
		  ObservedTier: viptiergate.TierStandard,
		   ExpectedTier: viptiergate.TierVIP,
		   ObservedBPS:  10,
		   ExpectedBPS:  5,
		   Amount: "1000.0",
		   Generation: 1,
	   })
	   if err != nil {
		   t.Fatalf("OpenDriftCase: %v", err)
	   }
	  // 时钟重置回当前。
	  svc.WithClock(fixedClock(now))
	  alert := &testAlertHook{}
	   w := viptiergate.NewWorker(svc, viptiergate.WorkerConfig{
		   FreezeScanInterval: 5 * time.Millisecond,
		  DriftScanInterval:     5 * time.Millisecond,
		   QuoteExpiryInterval:   5 * time.Millisecond,
		  LeaseProgressInterval: 5 * time.Millisecond,
		   EscalateThreshold:     10 * time.Minute, // 10 分钟阈值
		   Alert: alert,
	   }, nil)
	  w.Start(ctx)
	   time.Sleep(50 * time.Millisecond)
	   w.Stop()
	  // 应触发至少一次告警。
	  if alert.count.Load() == 0 {
		  t.Error("expected at least one escalation alert for aged open drift case")
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// Worker ScanBatchSize 限制
// ─────────────────────────────────────────────────────────────────────────────

   func TestWorker_ScanBatchSize(t *testing.T) {
	   ctx := context.Background()
	  svc := viptiergate.NewService(nil)
	   // 开立 5 个漂移案（超过 batch=3）。
	  for i := 0; i < 5; i++ {
		  uid := "u-batch-" + string(rune('A'+i))
		   svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
			   UserID: uid,
			   Asset:        "USDT",
			  Kind:         viptiergate.DriftBPSStale,
			   ObservedTier: viptiergate.TierStandard,
			   ExpectedTier: viptiergate.TierVIP,
			  ObservedBPS: 10,
			  ExpectedBPS: 5,
			  Amount:      "100.0",
			   Generation: 1,
		  })
	  }
	  w := viptiergate.NewWorker(svc, viptiergate.WorkerConfig{
		  FreezeScanInterval: 10 * time.Millisecond,
		   DriftScanInterval: 10 * time.Millisecond,
		   QuoteExpiryInterval:   10 * time.Millisecond,
		  LeaseProgressInterval: 10 * time.Millisecond,
		   ScanBatchSize:         3, // 批量限制 3
	  }, nil)
	  // 运行后不应 panic（批量截断正常处理）。
	   w.Start(ctx)
	  time.Sleep(60 * time.Millisecond)
	   w.Stop()
  }
