-- 链重组资金影响会计——审计流水与禁二次冲正告警表。
-- 此迁移依赖 0068_aa_reorg_acct.sql 已执行（impact_rows 表存在）。
-- 不改写任何 0068 已有表结构。
-- 作者：TianpengLan

-- ────────────────────────────────────────────────────────────────────────────
-- reorg_audit_log: 不可变操作审计流水
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reorg_audit_log (
  id          VARCHAR(64)    NOT NULL PRIMARY KEY,
  -- object_type: event / impact_row
  object_type VARCHAR(32)    NOT NULL DEFAULT '',
  object_id   VARCHAR(128)   NOT NULL DEFAULT '',
  from_state  VARCHAR(64)    NOT NULL DEFAULT '',
  to_state    VARCHAR(64)    NOT NULL DEFAULT '',
  -- operator: worker / hook / ops / system / scanner
  operator    VARCHAR(64)    NOT NULL DEFAULT '',
  -- source: confirm_rollback / orphan_observe / manual / timeout / guard
  source      VARCHAR(64)    NOT NULL DEFAULT '',
  note        VARCHAR(512)   NOT NULL DEFAULT '',
  at          DATETIME(3)    NOT NULL,

  KEY idx_reorg_audit_log_obj  (object_type, object_id, at),
  KEY idx_reorg_audit_log_at   (at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='重组影响会计不可变审计流水';

-- ────────────────────────────────────────────────────────────────────────────
-- double_reverse_audit: 双冲告警专用表（DoubleReverseScanner 写入）
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS double_reverse_audit (
  id            VARCHAR(64)    NOT NULL PRIMARY KEY,
  reference_id  VARCHAR(128)   NOT NULL DEFAULT '',
  batch_count   INT            NOT NULL DEFAULT 0,
  batch_ids     VARCHAR(1024)  NOT NULL DEFAULT '',
  detected_at   DATETIME(3)    NOT NULL,

  -- 同一 reference_id 去重（防止重复记录同一双冲事件）
  UNIQUE KEY uq_double_reverse_ref (reference_id),
  KEY        idx_double_reverse_at (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='潜在双冲告警记录（DoubleReverseScanner产出）';

-- ────────────────────────────────────────────────────────────────────────────
-- 补充索引（优化常用查询路径）
-- ────────────────────────────────────────────────────────────────────────────

-- 优化 FaceCompatStore.GetBatchState 查询（reverse_batches.id 已是 PK，无需额外索引）。
-- 优化 DoubleReverseScanner.scan 的 GROUP BY reference_id 查询。
ALTER TABLE impact_rows ADD INDEX IF NOT EXISTS idx_impact_rows_ref_batch (reference_id, reverse_batch_id);
