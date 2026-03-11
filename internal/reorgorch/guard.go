package reorgorch

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

// ────────────────────────────────────────────────────────────────────────────
// Prometheus 护栏指标
// ────────────────────────────────────────────────────────────────────────────

var (
	metricGuardConflicts = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name:      "guard_conflicts_total",
		Help: "Total guard conflicts (duplicate reverse attempt blocked).",
	})
	metricGuardOrphans = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name: "guard_orphan_rows_total",
		Help: "Total orphan ImpactRows detected by GuardScanner (no guard cover).",
	})
)

// ────────────────────────────────────────────────────────────────────────────
// GuardManager 护栏管理器
// ────────────────────────────────────────────────────────────────────────────
// GuardManager 负责护栏的写入与冲突检查。
type GuardManager struct {
	store *Store
	log zerolog.Logger
}

// NewGuardManager 构造 GuardManager。

   func NewGuardManager(store *Store, log zerolog.Logger) *GuardManager {
	   return &GuardManager{store: store, log: log}
  }

// Lock 尝试为 (impactRowID, legKind) 写入护栏。
// 若护栏已存在，返回 ErrGuardConflict；否则写入并返回 nil。
// live-hash 腿不写护栏（直接 skip）。
  func (gm *GuardManager) Lock(ctx context.Context, impactRowID, legKind, batchID, legID string) error {
	  if legKind == LegKindIncident {
		   // live-hash 事故单不写护栏（人工路径另行管理）。
		  return nil
	   }
	  existing,
		  err := gm.store.GetGuard(ctx, impactRowID, legKind)
	   if err != nil {
		  return fmt.Errorf("reorgorch guard lock query: %w", err)
	   }
	   if existing != nil {
		  metricGuardConflicts.Inc()
		   gm.log.Warn().
			  Str("impact_row_id", impactRowID).
			  Str("leg_kind", legKind).
			   Str("existing_batch_id", existing.BatchID).
			   Msg("reorgorch: guard conflict — duplicate reverse attempt blocked")
		  return ErrGuardConflict
	   }
	  guard := &ReverseGuard{
		   ID: uuid.New().String(),
		  ImpactRowID: impactRowID,
		   LegKind:  legKind,
		  BatchID:  batchID,
		   LegID:    legID,
		   LockedAt: time.Now().UTC(),
	   }
	  if err := gm.store.SaveGuard(ctx, guard); err != nil {
		   return fmt.Errorf("reorgorch guard lock save: %w", err)
	  }
	   return nil
  }

// Check 只读检查护栏是否存在（不写入）。

   func (gm *GuardManager) Check(ctx context.Context, impactRowID, legKind string) (bool, error) {
	   g, err := gm.store.GetGuard(ctx, impactRowID, legKind)
	  if err != nil {
		  return false, err
	  }
	  return g != nil, nil
   }

// ────────────────────────────────────────────────────────────────────────────
// GuardScanner 孤儿行巡检
// ────────────────────────────────────────────────────────────────────────────
// GuardScanner 定期巡检已关闭批次下是否有未被护栏覆盖的 ImpactRow（孤儿行）。
// 孤儿行意味着存在未完成冲正的资金位，需告警并人工复查。
type GuardScanner struct {
	store    *Store
	interval time.Duration
	log      zerolog.Logger
}

// NewGuardScanner 构造 GuardScanner。
  func NewGuardScanner(store *Store, interval time.Duration, log zerolog.Logger) *GuardScanner {
	  if interval <= 0 {
		  interval = 5 * time.Minute
	  }
	  return &GuardScanner{store: store, interval: interval, log: log}
  }

// Run 启动巡检循环，阻塞至 ctx.Done。
  func (gs *GuardScanner) Run(ctx context.Context) {
	   gs.log.Info().Dur("interval", gs.interval).Msg("reorgorch guard scanner started")
	   ticker := time.NewTicker(gs.interval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   gs.log.Info().Msg("reorgorch guard scanner stopping")
			   return
		   case <-ticker.C:
			  gs.scan(ctx)
		   }
	  }
  }

// scan 执行一轮孤儿行巡检。

  func (gs *GuardScanner) scan(ctx context.Context) {
	   orphans, err := gs.store.ListOrphanImpactRows(ctx, 100)
	  if err != nil {
		   gs.log.Error().Err(err).Msg("reorgorch guard scanner: list orphans failed")
		  return
	   }
	   if len(orphans) == 0 {
		   return
	   }
	  metricGuardOrphans.Add(float64(len(orphans)))
	  gs.log.Error().
		   Int("orphan_count", len(orphans)).
		  Msg("reorgorch: orphan ImpactRows detected — impact rows missing guard cover; manual review required")
	   // 将孤儿行写入审计表（不自动触发冲正）。
	   for _, rowID := range orphans {
		  if err := gs.store.RecordOrphanAudit(ctx, rowID); err != nil {
			   gs.log.Error().Err(err).Str("row_id", rowID).
				  Msg("reorgorch guard scanner: record orphan audit failed")
		  }
	   }
   }
