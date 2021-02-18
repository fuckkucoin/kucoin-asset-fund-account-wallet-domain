package availcas

import (
	"context"
	"time"
)

// WorkerConfig Worker 配置参数。
type WorkerConfig struct {
	// SweepInterval 租约回收轮间隔；默认 15s。
	SweepInterval time.Duration
	// SweepBatch 每轮最多回收租约数；默认 200。
	SweepBatch int
	// ConflictScanInterval 冲突扫描轮间隔；默认 60s。
	ConflictScanInterval time.Duration
	// ProofCheckInterval 证明完整性巡检轮间隔；默认 300s。
	ProofCheckInterval time.Duration
	// ProofCheckBatch 每轮最多巡检证明数；默认 100。
	ProofCheckBatch int
}

// DefaultWorkerConfig 返回默认 Worker 配置。
  func DefaultWorkerConfig() WorkerConfig {
	   return WorkerConfig{
		   SweepInterval:        15 * time.Second,
		  SweepBatch:           200,
		   ConflictScanInterval: 60 * time.Second,
		   ProofCheckInterval: 300 * time.Second,
		   ProofCheckBatch: 100,
	   }
   }

// Worker 封装三条维护 goroutine。

type Worker struct {
	lm *leaseManager
	store Store
	cfg WorkerConfig
}

// NewWorker 构造 Worker。svc 必须已初始化。
   func NewWorker(svc *Service, cfg WorkerConfig) *Worker {
	  return &Worker{
		  lm:    svc.lm,
		  store: svc.store,
		   cfg: cfg,
	   }
  }

// Run 启动三个维护 goroutine，直到 ctx 取消后全部退出。
// 通常由 svcboot 或 cmd main 调用： go worker.Run(ctx)
  func (w *Worker) Run(ctx context.Context) {
	  go w.runSweep(ctx)
	  go w.runConflictScan(ctx)
	   go w.runProofCheck(ctx)
	   <-ctx.Done()
  }

// ─── 租约回收 ─────────────────────────────────────────────────────────────────

   func (w *Worker) runSweep(ctx context.Context) {
	  interval := w.cfg.SweepInterval
	  if interval <= 0 {
		   interval = 15 * time.Second
	  }
	  ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.doSweep(ctx)
		  }
	  }
  }

  func (w *Worker) doSweep(ctx context.Context) {
	  start := time.Now()
	  batch := w.cfg.SweepBatch
	  if batch <= 0 {
		   batch = 200
	  }
	  count, err := w.lm.SweepExpired(ctx, batch)
	  elapsed := time.Since(start)
	   metricWorkerSweepDuration.Observe(elapsed.Seconds())
	  if err != nil {
		   // 回收 worker 失败不影响主路径；仅记录指标。
		   metricLeaseExpireErrors.Inc()
		   return
	  }
	   _ = count
  }

// ─── 冲突扫描 ─────────────────────────────────────────────────────────────────
// runConflictScan 定期扫描孤儿意图：
// accepted 但对应租约已 expired/revoked 的意图（账本可能未实际执行）。
// 只做告警与指标，不自动回滚（回滚由业务层负责）。

   func (w *Worker) runConflictScan(ctx context.Context) {
	   interval := w.cfg.ConflictScanInterval
	   if interval <= 0 {
		  interval = 60 * time.Second
	  }
	   ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			  w.doConflictScan(ctx)
		  }
	  }
   }

   func (w *Worker) doConflictScan(ctx context.Context) {
	  start := time.Now()
	   // 查询 accepted 意图，检查其对应租约是否已非 open/bound。
	   // 孤儿意图意味着账本操作可能已通过但租约已失效：
	   // 须人工介入确认账本实际状态，不自动变更意图或租约。
	   orphans, err := w.store.(*SQLStore).findOrphanIntents(ctx, 100)
	   if err != nil {
		   metricLeaseExpireErrors.Inc()
		  return
	   }
	  elapsed := time.Since(start)
	   metricWorkerConflictScanDuration.Observe(elapsed.Seconds())
	   // 孤儿意图只记录指标，不做任何自动处理，等待人工或业务层主动关闭。
	  for range orphans {
		   // 每个孤儿意图递增一次 conflict_scan 事件指标。
		  // 业务中应写事件日志或告警通道。
		  metricLeaseExpireErrors.Inc()
	  }
  }

// findOrphanIntents 查询孤儿意图（accepted 但租约非 open/bound）。
// 注意：只有 SQLStore 能做此查询；MemStore 不支持，跳过。

   func (s *SQLStore) findOrphanIntents(ctx context.Context, limit int) ([]*DebitIntent, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	   q := `SELECT di.id, di.idem_key, di.user_id, di.asset, di.book, 
	             di.amount, di.kind, di.reason,
	             di.lease_id, di.lease_gen, di.expected_avail,
	             di.state, di.reject_code, di.reject_note, di.race_proof_id,
	             di.actor_id, di.request_ip, di.proposed_at, di.decided_at
	      FROM debit_intents di
	      JOIN avail_leases al ON al.id = di.lease_id
	      WHERE di.state = 'accepted'
	        AND al.state NOT IN ('open','bound')
	      ORDER BY di.proposed_at ASC
	      LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, limit)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  var out []*DebitIntent
	  for rows.Next() {
		   d, err := scanIntent(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, d)
	  }
	   return out, rows.Err()
   }

// ─── 证明完整性巡检 ────────────────────────────────────────────────────────────
// runProofCheck 定期巡检竞态证明完整性。
// 检查口径：
// - loser_intent_id 非空且对应意图 state=rejected。
// - winner_intent_id 若非空则对应意图 state=accepted。
// - reject_code 非空。
  func (w *Worker) runProofCheck(ctx context.Context) {
	   interval := w.cfg.ProofCheckInterval
	  if interval <= 0 {
		  interval = 300 * time.Second
	   }
	   ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			  w.doProofCheck(ctx)
		   }
	   }
  }

  func (w *Worker) doProofCheck(ctx context.Context) {
	   batch := w.cfg.ProofCheckBatch
	   if batch <= 0 {
		  batch = 100
	   }
	   sqlStore, ok := w.store.(*SQLStore)
	  if !ok {
		  return
	   }
	   proofs, err := sqlStore.listRecentProofs(ctx, batch)
	   if err != nil {
		   metricProofIntegrityErrors.Inc()
		   return
	  }
	   for _, p := range proofs {
		  if err := w.checkProofIntegrity(ctx, p); err != nil {
			  metricProofIntegrityErrors.Inc()
		   }
	  }
   }

// listRecentProofs 查询最近 N 条证明（完整性巡检用）。
   func (s *SQLStore) listRecentProofs(ctx context.Context, limit int) ([]*RaceProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	  q := `SELECT id, user_id, asset, book, lease_id,
	             winner_intent_id, loser_intent_id,
	             winner_amount, loser_amount, avail_at_decision,
	             window_start, window_end, reject_code, note, created_at
	      FROM race_proofs
	      ORDER BY created_at DESC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, q, limit)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []*RaceProof
	   for rows.Next() {
		  p, err := scanProof(rows)
		   if err != nil {
			   return nil, err
		  }
		   out = append(out, p)
	  }
	  return out, rows.Err()
  }

// checkProofIntegrity 对单个 RaceProof 做一致性检查。

   func (w *Worker) checkProofIntegrity(ctx context.Context, p *RaceProof) error {
	  if p.RejectCode == "" {
		  return ErrProofNotFound
	  }
	  if p.LoserIntentID == "" {
		  return ErrProofNotFound
	   }
	   loser, err := w.store.GetIntent(ctx, p.LoserIntentID)
	   if err != nil {
		   return err
	  }
	  if loser.State != IntentStateRejected {
		  return ErrIntentFinalized
	  }
	   if p.WinnerIntentID != "" {
		  winner,
			  err := w.store.GetIntent(ctx, p.WinnerIntentID)
		   if err != nil {
			   return err
		   }
		  if winner.State != IntentStateAccepted {
			   return ErrIntentFinalized
		  }
	   }
	  return nil
   }
