package reorgacct

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

// ────────────────────────────────────────────────────────────────────────────
// Prometheus 审计指标
// ────────────────────────────────────────────────────────────────────────────

var (
	metricAuditEntriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "reorgacct",
		Name: "audit_entries_total",
		Help: "Total audit log entries written.",
	}, []string{"object_type"}) // object_type: event / impact_row
	metricDoubleReverseDetected = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgacct",
		Name: "double_reverse_detected_total",
		Help: "Total suspected double-reverse incidents detected.",
	})
)

// ────────────────────────────────────────────────────────────────────────────
// AuditEntry 审计流水行
// ────────────────────────────────────────────────────────────────────────────
// AuditObjectType 审计对象类型。
type AuditObjectType string

const (
	// AuditObjEvent ReorgEvent 状态变更审计。
	AuditObjEvent AuditObjectType = "event"
	// AuditObjImpactRow ImpactRow 批次绑定审计。
	AuditObjImpactRow AuditObjectType = "impact_row"
)

// AuditEntry 一条不可变的操作流水。
type AuditEntry struct {
	// ID UUID v4，主键。
	ID string `json:"id"`
	// ObjectType 审计对象类型（event / impact_row）。
	ObjectType AuditObjectType `json:"object_type"`
	// ObjectID 被审计对象的 ID。
	ObjectID string `json:"object_id"`
	// FromState 操作前状态（可为空，如首次创建）。
	FromState string `json:"from_state,omitempty"`
	// ToState 操作后状态。
	ToState string `json:"to_state"`
	// Operator 操作者标识（worker/hook/ops/system）。
	Operator string `json:"operator"`
	// Source 触发来源（confirm_rollback/orphan_observe/manual/timeout/guard）。
	Source string `json:"source"`
	// Note 操作说明。
	Note string `json:"note,omitempty"`
	// At 操作时间（UTC）。
	At time.Time `json:"at"`
}

// ────────────────────────────────────────────────────────────────────────────
// AuditLogger 审计流水写入器
// ────────────────────────────────────────────────────────────────────────────
// AuditLogger 负责将状态变更写入 reorg_audit_log 表（只增不改）。

type AuditLogger struct {
	db *sql.DB
}

// NewAuditLogger 构造 AuditLogger；db 不得为 nil。
   func NewAuditLogger(db *sql.DB) *AuditLogger {
	   return &AuditLogger{db: db}
   }

// Log 写入一条审计流水；非阻塞写入（调用方不应因审计失败中断业务流程）。
  func (al *AuditLogger) Log(ctx context.Context, entry AuditEntry) error {
	   if al == nil || al.db == nil {
		   return ErrStoreUnavailable
	  }
	  if entry.ID == "" {
		   entry.ID = uuid.New().String()
	  }
	   if entry.At.IsZero() {
		   entry.At = time.Now().UTC()
	   }
	   q := `INSERT INTO reorg_audit_log
		(id, object_type, object_id, from_state, to_state, operator, source, note, at)
		VALUES (?,?,?,?,?,?,?,?,?)`
	  _,
		  err := al.db.ExecContext(ctx, q,
		  entry.ID, string(entry.ObjectType), entry.ObjectID,
		  entry.FromState, entry.ToState, entry.Operator, entry.Source,
		   entry.Note, entry.At,
	   )
	  if err != nil {
		  return fmt.Errorf("reorgacct audit log: %w", err)
	   }
	   metricAuditEntriesTotal.WithLabelValues(string(entry.ObjectType)).Inc()
	   return nil
   }

// LogEventTransition 记录 ReorgEvent 状态跃迁。
   func (al *AuditLogger) LogEventTransition(ctx context.Context, eventID, fromState, toState, operator, source, note string) {
	   _ = al.Log(ctx, AuditEntry{
		  ObjectType: AuditObjEvent,
		   ObjectID:  eventID,
		   FromState: fromState,
		  ToState:  toState,
		  Operator: operator,
		   Source:   source,
		   Note: note,
		  At:   time.Now().UTC(),
	  })
  }

// LogImpactRowBind 记录 ImpactRow 批次绑定操作。
  func (al *AuditLogger) LogImpactRowBind(ctx context.Context, rowID, batchID, operator string) {
	  _ = al.Log(ctx, AuditEntry{
		   ObjectType: AuditObjImpactRow,
		   ObjectID:  rowID,
		  FromState: "",
		  ToState:  "bound:" + batchID,
		   Operator: operator,
		  Source:   "orchestrator",
		  Note: fmt.Sprintf("impact row bound to batch %s", batchID),
		   At:   time.Now().UTC(),
	   })
  }

// ListAuditEntries 按对象 ID 查询审计流水（新的在前）。
  func (al *AuditLogger) ListAuditEntries(ctx context.Context, objectID string, limit int) ([]AuditEntry, error) {
	   if al == nil || al.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	  if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	   q := `SELECT id, object_type, object_id, COALESCE(from_state,''), to_state,
	             operator, source, COALESCE(note,''), at
	      FROM reorg_audit_log WHERE object_id = ? ORDER BY at DESC LIMIT ?`
	  rows, err := al.db.QueryContext(ctx, q, objectID, limit)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []AuditEntry
	  for rows.Next() {
		   var e AuditEntry
		  var objType string
		   if err := rows.Scan(&e.ID, &objType, &e.ObjectID,
			   &e.FromState, &e.ToState, &e.Operator, &e.Source, &e.Note, &e.At); err != nil {
			  return nil, err
		  }
		   e.ObjectType = AuditObjectType(objType)
		   out = append(out, e)
	   }
	  return out, rows.Err()
  }

// ────────────────────────────────────────────────────────────────────────────
// DoubleReverseScanner 禁二次巡检
// ────────────────────────────────────────────────────────────────────────────
// DoubleReverseScanner 定期扫描已完成冲正的 ImpactRow，
// 检测是否存在多个 reverse_batch_id 被绑定的情况（潜在双冲）。

type DoubleReverseScanner struct {
	db *sql.DB
	audit    *AuditLogger
	interval time.Duration
	log      zerolog.Logger
}

// NewDoubleReverseScanner 构造 DoubleReverseScanner。

  func NewDoubleReverseScanner(db *sql.DB, audit *AuditLogger, interval time.Duration, log zerolog.Logger) *DoubleReverseScanner {
	   if interval <= 0 {
		  interval = 10 * time.Minute
	  }
	  return &DoubleReverseScanner{db: db,
		  audit: audit, interval: interval, log: log}
   }

// Run 启动巡检循环，阻塞至 ctx.Done。
  func (sc *DoubleReverseScanner) Run(ctx context.Context) {
	   sc.log.Info().Dur("interval", sc.interval).Msg("reorgacct double-reverse scanner started")
	  ticker := time.NewTicker(sc.interval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			  sc.scan(ctx)
		   }
	   }
  }

// scan 执行一轮双冲扫描。
// 查询：同一 reference_id 存在多条 reverse_batch_id 不同且均非空的 impact_row。

   func (sc *DoubleReverseScanner) scan(ctx context.Context) {
	   if sc.db == nil {
		  return
	   }
	  // 按 reference_id 聚合 distinct reverse_batch_id，找到 count > 1 的行。
	   q := `SELECT reference_id, COUNT(DISTINCT reverse_batch_id) AS batch_count, 
	             GROUP_CONCAT(DISTINCT reverse_batch_id) AS batch_ids
	      FROM impact_rows
	      WHERE reverse_batch_id IS NOT NULL AND reverse_batch_id != ''
	      GROUP BY reference_id
	      HAVING batch_count > 1
	      LIMIT 50`
	   rows, err := sc.db.QueryContext(ctx, q)
	  if err != nil {
		   sc.log.Error().Err(err).Msg("reorgacct double-reverse scanner: query failed")
		  return
	   }
	   defer rows.Close()
	   for rows.Next() {
		   var refID, batchIDs string
		  var batchCount int
		   if err := rows.Scan(&refID, &batchCount, &batchIDs); err != nil {
			  continue
		   }
		   metricDoubleReverseDetected.Inc()
		  sc.log.Error().
			  Str("reference_id", refID).
			   Int("batch_count", batchCount).
			   Str("batch_ids", batchIDs).
			   Msg("reorgacct: DOUBLE-REVERSE DETECTED — same reference reversed in multiple batches")
			  // 写入审计记录。
		  _ = sc.audit.Log(ctx, AuditEntry{
			  ObjectType: AuditObjImpactRow,
			   ObjectID: refID,
			  FromState: "",
			   ToState:   "double_reverse_detected",
			   Operator: "double_reverse_scanner",
			  Source: "scanner",
			   Note:   fmt.Sprintf("reference_id=%s found in %d batches: %s", refID, batchCount, batchIDs),
			  At:     time.Now().UTC(),
		  })
		  // 写入 double_reverse_audit 表（专用告警表）。
		   _ = sc.recordDoubleReverse(ctx, refID, batchCount, batchIDs)
	   }
   }

// recordDoubleReverse 写入专用双冲告警记录。

   func (sc *DoubleReverseScanner) recordDoubleReverse(ctx context.Context, refID string, batchCount int, batchIDs string) error {
	  q := `INSERT IGNORE INTO double_reverse_audit
		(id, reference_id, batch_count, batch_ids, detected_at)
		VALUES (?, ?, ?, ?, ?)`
	   _, err := sc.db.ExecContext(ctx, q, uuid.New().String(), refID, batchCount, batchIDs, time.Now().UTC())
	  _ = err
	  // 任何写入错误不影响巡检接着；指标已记录。
	   return nil
   }
