package inflightgengate

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
	"github.com/rs/zerolog"
)

// WorkerMetrics worker 可观测性接口（可注入 Prometheus counter）。

type WorkerMetrics interface {
	IncDriftCasePatrolled(n int)
	IncRefLeakDetected(n int)
	IncRevokeCompleted(n int)
	IncWorkerError(worker string)
}

// noopWorkerMetrics 空指标实现。

type noopWorkerMetrics struct{}

  func (noopWorkerMetrics) IncDriftCasePatrolled(int) {}
   func (noopWorkerMetrics) IncRefLeakDetected(int)    {}
   func (noopWorkerMetrics) IncRevokeCompleted(int)    {}

   func (noopWorkerMetrics) IncWorkerError(string) {}

// Workers 聚合所有后台 worker。
type Workers struct {
	patrol *DriftPatrolWorker
	leak     *RefLeakScanWorker
	complete *RevokeCompleteWorker
}

// NewWorkers 构造聚合 worker。

   func NewWorkers(eng *destlife.InflightEngine, cfg GateConfig, log zerolog.Logger, m WorkerMetrics) *Workers {
	   if m == nil {
		   m = noopWorkerMetrics{}
	   }
	  return &Workers{
		   patrol:   newDriftPatrolWorker(eng, cfg, log, m),
		   leak:     newRefLeakScanWorker(eng, cfg, log, m),
		  complete: newRevokeCompleteWorker(eng, cfg, log, m),
	   }
   }

// RunAll 并发启动所有 worker，直到 ctx 取消。
   func (w *Workers) RunAll(ctx context.Context) {
	  done := make(chan struct{}, 3)
	   run := func(fn func(context.Context) error, name string) {
		  defer func() { done <- struct{}{} }()
		  if err := fn(ctx); err != nil && ctx.Err() == nil {
			   fmt.Printf("[inflightgengate][%s] worker stopped: %v\n", name, err)
		  }
	  }
	  go run(w.patrol.Run, "drift-patrol")
	  go run(w.leak.Run, "ref-leak-scan")
	   go run(w.complete.Run, "revoke-complete")
	  for i := 0; i < 3; i++ {
		   <-done
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// DriftPatrolWorker
// ─────────────────────────────────────────────────────────────────────────────
// DriftPatrolWorker 漂移案例巡检 worker。

type DriftPatrolWorker struct {
	eng *destlife.InflightEngine
	cfg     GateConfig
	log     zerolog.Logger
	metrics WorkerMetrics
}

   func newDriftPatrolWorker(eng *destlife.InflightEngine, cfg GateConfig, log zerolog.Logger, m WorkerMetrics) *DriftPatrolWorker {
	  return &DriftPatrolWorker{eng: eng,
		   cfg: cfg, log: log, metrics: m}
   }

// Run 启动巡检循环，直到 ctx 取消。

   func (w *DriftPatrolWorker) Run(ctx context.Context) error {
	  t := time.NewTicker(w.cfg.driftPatrolInterval())
	   defer t.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return ctx.Err()
		   case <-t.C:
			   if err := w.RunOnce(ctx); err != nil {
				  w.metrics.IncWorkerError("drift-patrol")
				  w.log.Error().Err(err).Msg("inflightgengate: drift patrol scan failed")
			   }
		  }
	  }
   }

// RunOnce 单次漂移案例巡检。

   func (w *DriftPatrolWorker) RunOnce(ctx context.Context) error {
	  cases, err := w.eng.ListOpenDriftCases(ctx, w.cfg.batchLimit())
	  if err != nil {
		  return fmt.Errorf("list open drift cases: %w", err)
	   }
	   for _, dc := range cases {
		  w.log.Warn().
			  Str("case_id", dc.CaseID).
			  Str("entry_id", dc.EntryID).
			  Str("kind", string(dc.Kind)).
			   Str("movement_id", dc.MovementID).
			  Int64("inflight_gen", dc.InflightGen).
			   Int64("bound_revoke_gen", dc.BoundRevokeGen).
			  Int64("current_revoke_gen", dc.CurrentRevokeGen).
			   Time("opened_at", dc.OpenedAt).
			   Msg("inflightgengate: open drift case detected")
	   }
	  w.metrics.IncDriftCasePatrolled(len(cases))
	   return nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// RefLeakScanWorker
// ─────────────────────────────────────────────────────────────────────────────
// RefLeakScanWorker 引用泄漏扫描 worker。
type RefLeakScanWorker struct {
	eng *destlife.InflightEngine
	cfg GateConfig
	log zerolog.Logger
	metrics WorkerMetrics
}

   func newRefLeakScanWorker(eng *destlife.InflightEngine, cfg GateConfig, log zerolog.Logger, m WorkerMetrics) *RefLeakScanWorker {
	  return &RefLeakScanWorker{eng: eng, cfg: cfg, log: log, metrics: m}
  }

// Run 启动泄漏扫描循环，直到 ctx 取消。

  func (w *RefLeakScanWorker) Run(ctx context.Context) error {
	  t := time.NewTicker(w.cfg.leakScanInterval())
	   defer t.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-t.C:
			   if err := w.RunOnce(ctx); err != nil {
				   w.metrics.IncWorkerError("ref-leak-scan")
				   w.log.Error().Err(err).Msg("inflightgengate: ref leak scan failed")
			  }
		   }
	  }
  }

// RunOnce 单次泄漏扫描：发现超时未释放引用，产出泄漏漂移案例。

  func (w *RefLeakScanWorker) RunOnce(ctx context.Context) error {
	  leases, err := w.eng.ListExpiredLeases(ctx, w.cfg.batchLimit())
	   if err != nil {
		  return fmt.Errorf("list expired leases: %w", err)
	   }
	  leaked := 0
	  for _, l := range leases {
		  w.log.Warn().
			   Str("lease_id", l.LeaseID).
			  Str("entry_id", l.EntryID).
			   Str("movement_id", l.MovementID).
			  Str("state", string(l.State)).
			   Time("expires_at", l.ExpiresAt).
			  Msg("inflightgengate: ref leak detected")
		  dc, dcErr := w.eng.GetOpenDriftCase(ctx, l.EntryID)
		   if dcErr == nil && dc.Kind == destlife.DriftRefLeakTimeout {
			   continue
		   }
		   leaked++
	  }
	   w.metrics.IncRefLeakDetected(leaked)
	   return nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// RevokeCompleteWorker
// ─────────────────────────────────────────────────────────────────────────────
// RevokeCompleteWorker 吊销完成 worker。
// 扫描 state=revoking 的条目，若活跃引用归零则完成吊销。
type RevokeCompleteWorker struct {
	eng *destlife.InflightEngine
	cfg GateConfig
	log     zerolog.Logger
	metrics WorkerMetrics
}

  func newRevokeCompleteWorker(eng *destlife.InflightEngine, cfg GateConfig, log zerolog.Logger, m WorkerMetrics) *RevokeCompleteWorker {
	  return &RevokeCompleteWorker{eng: eng, cfg: cfg, log: log, metrics: m}
   }

// Run 启动吊销完成循环，直到 ctx 取消。

  func (w *RevokeCompleteWorker) Run(ctx context.Context) error {
	   t := time.NewTicker(w.cfg.revokeCompleteInterval())
	   defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return ctx.Err()
		   case <-t.C:
			  if err := w.RunOnce(ctx); err != nil {
				  w.metrics.IncWorkerError("revoke-complete")
				   w.log.Error().Err(err).Msg("inflightgengate: revoke complete scan failed")
			   }
		   }
	   }
   }

// RunOnce 单次吊销完成扫描。
  func (w *RevokeCompleteWorker) RunOnce(ctx context.Context) error {
	   revoking, err := w.eng.ListRevokingEntries(ctx, w.cfg.batchLimit())
	  if err != nil {
		   return fmt.Errorf("list revoking entries: %w", err)
	  }
	   completed := 0
	  for _, rg := range revoking {
		  active, countErr := w.eng.ListActiveRefs(ctx, rg.EntryID)
		   if countErr != nil {
			  w.log.Error().Err(countErr).Str("entry_id", rg.EntryID).Msg("inflightgengate: count active refs failed")
			  continue
		   }
		   if len(active) == 0 {
			  if _, rErr := w.eng.GetRevokeGeneration(ctx, rg.EntryID); rErr != nil {
				   continue
			  }
			  w.log.Info().
				  Str("entry_id", rg.EntryID).
				   Int64("gen", rg.Gen).
				  Msg("inflightgengate: revoke complete: all refs released")
			  completed++
		  } else {
			   w.log.Debug().
				  Str("entry_id", rg.EntryID).
				   Int("active_refs", len(active)).
				  Msg("inflightgengate: revoke deferred: active refs remain")
		   }
	   }
	   w.metrics.IncRevokeCompleted(completed)
	   return nil
   }
