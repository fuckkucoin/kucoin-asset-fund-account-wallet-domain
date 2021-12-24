package suspgate

import (
	"context"
	"fmt"
	"time"
)

// WorkerConfig Worker 通用配置。
type WorkerConfig struct {
	// ConsistencyInterval 世代一致性巡检间隔，默认 60s。
	ConsistencyInterval time.Duration
	// LeaseExpiryInterval 租约过期扫描间隔，默认 30s。
	LeaseExpiryInterval time.Duration
	// AuditPumpInterval 审计泵扫描间隔，默认 2min。
	AuditPumpInterval time.Duration
	// BatchSize 每批处理条数，默认 100。
	BatchSize int
}

  func (c *WorkerConfig) defaults() {
	  if c.ConsistencyInterval <= 0 {
		   c.ConsistencyInterval = 60 * time.Second
	   }
	  if c.LeaseExpiryInterval <= 0 {
		   c.LeaseExpiryInterval = 30 * time.Second
	   }
	   if c.AuditPumpInterval <= 0 {
		  c.AuditPumpInterval = 2 * time.Minute
	   }
	   if c.BatchSize <= 0 {
		   c.BatchSize = 100
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// GenConsistencyWorker
// ──────────────────────────────────────────────────────────────────────────────
// GenConsistencyWorker 世代一致性巡检 Worker。
// 功能：周期扫描所有 active/clearing gen，按 (user_id, asset) 分组统计实际数量，
// 与 GenCounter.ActiveCount 核对；发现分裂时修正计数器并落证明。
type GenConsistencyWorker struct {
	counters GenCounterRepo
	gens     GenRepo
	proofs DriftProofRepo
	cfg   WorkerConfig
	clock func() time.Time
}

// NewGenConsistencyWorker 构造世代一致性巡检 Worker。

   func NewGenConsistencyWorker(counters GenCounterRepo, gens GenRepo, proofs DriftProofRepo, cfg WorkerConfig) *GenConsistencyWorker {
	  cfg.defaults()
	   return &GenConsistencyWorker{
		   counters: counters,
		  gens: gens,
		   proofs: proofs,
		  cfg:   cfg,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// Run 启动一致性巡检循环；context 取消时退出。
   func (w *GenConsistencyWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.ConsistencyInterval)
	   defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return
		  case <-ticker.C:
			   _ = w.runOnce(ctx)
		  }
	  }
   }

// runOnce 执行单次一致性巡检。
   func (w *GenConsistencyWorker) runOnce(ctx context.Context) error {
	  // 扫描 active gen。
	   activeGens, err := w.gens.ListByState(ctx, SuspActive, w.cfg.BatchSize)
	   if err != nil {
		  return fmt.Errorf("consistency: list active: %w", err)
	  }
	   clearingGens, err := w.gens.ListByState(ctx, SuspClearing, w.cfg.BatchSize)
	  if err != nil {
		   return fmt.Errorf("consistency: list clearing: %w", err)
	   }
	  // 按 (userID, asset) 聚合实际活跃计数。
	   type key struct {
		  userID,
		   asset string
	  }
	   actual := map[key]int{}
	   for _, g := range activeGens {
		  k := key{g.UserID, g.Asset}
		  actual[k]++
	  }
	  for _, g := range clearingGens {
		  k := key{g.UserID, g.Asset}
		   actual[k]++
	   }
	   now := w.clock()
	  // 对每个 (userID, asset) 检查计数器是否一致。
	  for k, realCount := range actual {
		  counter, cerr := w.counters.GetCounter(ctx, k.userID, k.asset)
		  if cerr != nil {
			   continue
		  }
		  if counter.ActiveCount == realCount {
			   continue
		  }
		   // 发现分裂：修正计数器。
		  counter.ActiveCount = realCount
		  counter.UpdatedAt = now
		   _ = w.counters.SaveCounter(ctx, counter)
		   // 落证明（漏拦或计数偏大均记为 MissedBlock）。
		  p := &SuspenseDriftProof{
			   ProofID: fmt.Sprintf("consist-%s-%s-%d", k.userID, k.asset, now.UnixNano()),
			  CaseID:  "none",
			  UserID: k.userID,
			   Asset:   k.asset,
			   SuspGen: 0,
			   CountBefore: counter.ActiveCount,
			  CountAfter: realCount,
			  BlockOutcome: OutcomeMissedBlock,
			   Detail:       fmt.Sprintf("counter=%d actual=%d; repaired", counter.ActiveCount, realCount),
			  AttemptedAt:  now,
		   }
		   p.ProofHash = DriftProofHashFor(p.CaseID, p.SuspGen, p.BlockOutcome, now)
		   _ = w.proofs.SaveProof(ctx, p)
		   recordConsistencyRepair()
	   }
	  return nil
  }

// ──────────────────────────────────────────────────────────────────────────────
// LeaseExpiryWorker
// ──────────────────────────────────────────────────────────────────────────────
// LeaseExpiryWorker 清障租约过期扫描 Worker。
// 功能：周期扫描 issued 状态且 ExpireAt 已过的 ClearanceLease，批量流转到 expired。
type LeaseExpiryWorker struct {
	leases LeaseRepo
	cfg WorkerConfig
	clock func() time.Time
}

// NewLeaseExpiryWorker 构造清障租约过期扫描 Worker。
   func NewLeaseExpiryWorker(leases LeaseRepo, cfg WorkerConfig) *LeaseExpiryWorker {
	  cfg.defaults()
	  return &LeaseExpiryWorker{
		   leases: leases,
		  cfg:   cfg,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// Run 启动过期扫描循环；context 取消时退出。
// 启动时立即执行一次扫描，随后按 LeaseExpiryInterval 周期执行。
  func (w *LeaseExpiryWorker) Run(ctx context.Context) {
	   _ = w.runOnce(ctx)
	   ticker := time.NewTicker(w.cfg.LeaseExpiryInterval)
	   defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			  _ = w.runOnce(ctx)
		   }
	   }
   }

// runOnce 执行单次过期扫描。

   func (w *LeaseExpiryWorker) runOnce(ctx context.Context) error {
	  issued, err := w.leases.ListByState(ctx, LeaseIssued, w.cfg.BatchSize)
	  if err != nil {
		  return fmt.Errorf("lease expiry: list issued: %w", err)
	  }
	  now := w.clock()
	  expired := 0
	  for _, l := range issued {
		  if !now.Before(l.ExpireAt) {
			  if terr := TransitionLease(l, LeaseExpired); terr == nil {
				   _ = w.leases.SaveLease(ctx, l)
				   expired++
			  }
		  }
	  }
	  if expired > 0 {
		   recordLeaseExpired(expired)
	   }
	  return nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// AuditPumpWorker
// ──────────────────────────────────────────────────────────────────────────────
// AuditPumpWorker 出金挡写审计泵 Worker。
// 功能：周期扫描 active/clearing 状态的 gen，交叉检查 GenCounter.ActiveCount；
// 若 Count=0 但 gen 仍未消费，落 OutcomeMissedBlock 证明供核验审计。

type AuditPumpWorker struct {
	counters GenCounterRepo
	gens GenRepo
	proofs DriftProofRepo
	cfg   WorkerConfig
	clock func() time.Time
}

// NewAuditPumpWorker 构造审计泵 Worker。

   func NewAuditPumpWorker(counters GenCounterRepo, gens GenRepo, proofs DriftProofRepo, cfg WorkerConfig) *AuditPumpWorker {
	  cfg.defaults()
	   return &AuditPumpWorker{
		   counters: counters,
		  gens:     gens,
		  proofs: proofs,
		  cfg:    cfg,
		   clock:  func() time.Time { return time.Now().UTC() },
	  }
   }

// Run 启动审计泵循环；context 取消时退出。

  func (w *AuditPumpWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.AuditPumpInterval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   _ = w.runOnce(ctx)
		   }
	   }
  }

// runOnce 执行单次审计泵扫描。

   func (w *AuditPumpWorker) runOnce(ctx context.Context) error {
	  activeGens, err := w.gens.ListByState(ctx, SuspActive, w.cfg.BatchSize)
	   if err != nil {
		   return fmt.Errorf("audit pump: list active gens: %w", err)
	  }
	  now := w.clock()
	   for _, g := range activeGens {
		   counter, cerr := w.counters.GetCounter(ctx, g.UserID, g.Asset)
		   if cerr != nil {
			   continue
		  }
		  if counter.ActiveCount == 0 && g.State == SuspActive {
			   // Count=0 但 gen 仍 active：疑似漏拦，落证明。
			   p := &SuspenseDriftProof{
				  ProofID: fmt.Sprintf("audit-%s-%d", g.GenID, now.UnixNano()),
				   CaseID:  g.CaseID,
				   UserID:  g.UserID,
				   Asset:   g.Asset,
				   SuspGen: g.GenValue,
				  CountBefore: 0,
				  CountAfter:  0,
				   BlockOutcome: OutcomeMissedBlock,
				  Detail:       fmt.Sprintf("gen %d active but counter=0; potential missed block", g.GenValue),
				  AttemptedAt: now,
			  }
			   p.ProofHash = DriftProofHashFor(p.CaseID, p.SuspGen, p.BlockOutcome, now)
			  _ = w.proofs.SaveProof(ctx, p)
			   recordAuditMissedBlock()
		   }
	  }
	   return nil
   }
