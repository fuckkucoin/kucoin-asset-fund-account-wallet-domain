package stablemintgate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/stablebr"
)

// WorkerConfig 后台泵配置。
type WorkerConfig struct {
	ExpiryInterval time.Duration
	ClearInterval time.Duration
	HalfInterval  time.Duration
	BatchSize     int
}

  func (c *WorkerConfig) defaults() {
	   if c.ExpiryInterval <= 0 {
		  c.ExpiryInterval = 30 * time.Second
	   }
	  if c.ClearInterval <= 0 {
		  c.ClearInterval = 45 * time.Second
	  }
	  if c.HalfInterval <= 0 {
		  c.HalfInterval = 40 * time.Second
	  }
	  if c.BatchSize <= 0 {
		   c.BatchSize = 100
	   }
   }

// ExpiryWorker 过期租约与过期证明扫描。

type ExpiryWorker struct {
	eng *Engine
	cfg WorkerConfig
}

  func NewExpiryWorker(eng *Engine, cfg WorkerConfig) *ExpiryWorker {
	   cfg.defaults()
	  return &ExpiryWorker{eng: eng, cfg: cfg}
   }

   func (w *ExpiryWorker) Run(ctx context.Context) {
	  if w == nil || w.eng == nil {
		   return
	  }
	  t := time.NewTicker(w.cfg.ExpiryInterval)
	  defer t.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-t.C:
			   _,
				  _ = w.eng.ScanExpired(ctx, w.cfg.BatchSize)
		   }
	  }
   }

  func (w *ExpiryWorker) TickOnce(ctx context.Context) (int, error) {
	   return w.eng.ScanExpired(ctx, w.cfg.BatchSize)
   }

// ClearRevokeWorker 复扫仍活但世代已抬升的租约（ClearAttest 泵补偿）。
type ClearRevokeWorker struct {
	eng *Engine
	cfg WorkerConfig
}

  func NewClearRevokeWorker(eng *Engine, cfg WorkerConfig) *ClearRevokeWorker {
	   cfg.defaults()
	   return &ClearRevokeWorker{eng: eng, cfg: cfg}
  }

  func (w *ClearRevokeWorker) Run(ctx context.Context) {
	  if w == nil || w.eng == nil {
		   return
	  }
	   t := time.NewTicker(w.cfg.ClearInterval)
	   defer t.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-t.C:
			  _, _ = w.eng.ScanGenerationDrift(ctx, w.cfg.BatchSize)
		   }
	  }
   }

   func (w *ClearRevokeWorker) TickOnce(ctx context.Context) (int, error) {
	  return w.eng.ScanGenerationDrift(ctx, w.cfg.BatchSize)
  }

// HalfInspectWorker 半截 Journal 巡检。

type HalfInspectWorker struct {
	eng *Engine
	cfg WorkerConfig
}

   func NewHalfInspectWorker(eng *Engine, cfg WorkerConfig) *HalfInspectWorker {
	  cfg.defaults()
	  return &HalfInspectWorker{eng: eng, cfg: cfg}
   }

   func (w *HalfInspectWorker) Run(ctx context.Context) {
	   if w == nil || w.eng == nil {
		  return
	  }
	  t := time.NewTicker(w.cfg.HalfInterval)
	  defer t.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  return
		  case <-t.C:
			   _, _ = w.eng.ScanOpenHalves(ctx, w.cfg.BatchSize)
		  }
	  }
  }

  func (w *HalfInspectWorker) TickOnce(ctx context.Context) (int, error) {
	  return w.eng.ScanOpenHalves(ctx, w.cfg.BatchSize)
   }

// StartWorkers 启动三泵。

  func StartWorkers(ctx context.Context, eng *Engine, cfg WorkerConfig) {
	   cfg.defaults()
	  go NewExpiryWorker(eng,
		   cfg).Run(ctx)
	   go NewClearRevokeWorker(eng, cfg).Run(ctx)
	   go NewHalfInspectWorker(eng, cfg).Run(ctx)
   }

// ScanExpired 将 TTL 到期或证明过期的活租约置 expired。
   func (e *Engine) ScanExpired(ctx context.Context, limit int) (int, error) {
	  if e == nil {
		  return 0, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 100
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   now := e.now()
	   rows, err := e.leases.ListLiveExpired(ctx, now, limit)
	  if err != nil {
		   return 0, err
	   }
	  n := 0
	  for _, l := range rows {
		  if err := TransitionLease(l, LeaseExpired); err != nil {
			   continue
		  }
		   if err := e.leases.Save(ctx, l); err != nil {
			  continue
		   }
		   _ = e.denyLocked(ctx, l.LeaseID, ScopeKey(l.Kind, l.Asset), l.Kind, l.Asset, OutcomeBlockedLeaseExp, "worker expiry")
		  recordExpired(string(l.Kind))
		  n++
	  }
	  // 证明 Expiry 已过但租约 TTL 未到：复验指纹。
	  issued, _ := e.leases.ListByState(ctx, LeaseIssued, limit)
	   posting, _ := e.leases.ListByState(ctx, LeasePosting, limit)
	   live := append(issued, posting...)
	   for _, l := range live {
		   if l == nil || !now.Before(l.ExpireAt) {
			  continue
		  }
		  if err := e.br.MatchFingerprint(l.Kind.BridgeKind(), l.Asset, l.Fingerprint.Digest); err != nil {
			   _ = TransitionLease(l, LeaseExpired)
			  _ = e.leases.Save(ctx, l)
			  _ = e.denyLocked(ctx, l.LeaseID, ScopeKey(l.Kind, l.Asset), l.Kind, l.Asset, OutcomeBlockedExpire, err.Error())
			  recordExpired(string(l.Kind))
			  n++
		  }
	  }
	   return n, nil
   }

// ScanGenerationDrift 世代仓吊销过期世代租约。

  func (e *Engine) ScanGenerationDrift(ctx context.Context, limit int) (int, error) {
	   if e == nil {
		   return 0, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 100
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   n := 0
	  for _, st := range []LeaseState{LeaseIssued, LeasePosting} {
		   rows,
			  err := e.leases.ListByState(ctx, st, limit)
		   if err != nil {
			   return n, err
		   }
		  for _, l := range rows {
			   g, _ := e.gens.Get(ctx, ScopeKey(l.Kind, l.Asset))
			   if g == nil || g.MintGen == l.MintGen {
				   continue
			   }
			  if err := e.revokeLocked(ctx, l, "worker-gen", "generation drift", OutcomeBlockedGen); err == nil {
				   n++
			  }
		  }
	  }
	   return n, nil
  }

// ScanOpenHalves 复述未关闭半截，写巡检审计。

   func (e *Engine) ScanOpenHalves(ctx context.Context, limit int) (int, error) {
	   if e == nil {
		   return 0, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 100
	   }
	  rows, err := e.halves.ListOpen(ctx, limit)
	   if err != nil {
		   return 0, err
	  }
	  n := 0
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   for _, p := range rows {
		  _ = e.auditLocked(ctx, p.LeaseID, ScopeKey(p.Kind, p.Asset), "worker-half", "inspect_half", p.Cause, string(OutcomeHalfOpen))
		  recordHalf("inspect")
		   n++
	   }
	   return n, nil
  }

// EnsureAttestItems 导出当前所需条目（只读，不改 Conserve）。
  func RequiredBridgeItems(kind LeaseKind) []stablebr.Item {
	   return stablebr.RequiredItems(kind.BridgeKind())
  }
