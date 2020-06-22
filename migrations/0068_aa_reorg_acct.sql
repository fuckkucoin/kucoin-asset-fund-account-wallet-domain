-- 链重组资金影响会计与自动冲正编排引擎。
-- 表前缀 reorg_ 与既有 aa_ 前缀保持区分；impact_ / reverse_ 子前缀区分两层。
-- 金额列 DECIMAL(36,18)；时间列 DATETIME(3)。
-- 作者：TianpengLan
-- JSON 列（affected_outputs / affected_tracks / evidence / impact_row_ids）
--   使用 MEDIUMTEXT 兼容 MySQL 5.7；不依赖 JSON 类型，避免旧版本不兼容。
-- 不改写 aa_wd_broadcasts / saga 任何已有列与约束。
-- 不改写 r1 守恒公式表、r4 风险占用表。

-- ────────────────────────────────────────────────────────────────────────────
-- reorg_events: 规范化重组事件主表
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reorg_events (
  id              VARCHAR(64)    NOT NULL PRIMARY KEY,
  chain           VARCHAR(16)    NOT NULL DEFAULT '',
  pivot_tx_hash   VARCHAR(128)   NOT NULL DEFAULT '',
  depth_before    INT            NOT NULL DEFAULT 0,
  depth_after     INT            NOT NULL DEFAULT 0,
  rollback_depth  INT            NOT NULL DEFAULT 0,
  -- JSON 数组：AffectedOutput[] / AffectedTrack[] / EvidenceRef[]
  affected_outputs MEDIUMTEXT    NOT NULL,
  affected_tracks  MEDIUMTEXT    NOT NULL,
  evidence         MEDIUMTEXT    NOT NULL,
  state           VARCHAR(32)    NOT NULL DEFAULT 'pending',
  source_kind     VARCHAR(32)    NOT NULL DEFAULT 'confirm_rollback',
  note            VARCHAR(512)   NOT NULL DEFAULT '',
  -- idem_key = SHA-256(chain|pivot_tx_hash|depth_before|depth_after)[:16]
  idem_key        VARCHAR(64)    NOT NULL DEFAULT '',
  created_at      DATETIME(3)    NOT NULL,
  updated_at      DATETIME(3)    NOT NULL,

  UNIQUE KEY uq_reorg_events_idem   (idem_key),
  KEY        idx_reorg_events_chain  (chain, state),
  KEY        idx_reorg_events_state  (state, created_at),
  KEY        idx_reorg_events_pivot  (pivot_tx_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='链重组规范化事件';

-- ────────────────────────────────────────────────────────────────────────────
-- impact_rows: 影响面明细表（reorgacct 盘点产出）
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS impact_rows (
  id              VARCHAR(64)    NOT NULL PRIMARY KEY,
  event_id        VARCHAR(64)    NOT NULL DEFAULT '',
  user_id         VARCHAR(64)    NOT NULL DEFAULT '',
  asset           VARCHAR(16)    NOT NULL DEFAULT '',
  -- kind: credited / occupied / freeze / hop_partial / live_hash
  kind            VARCHAR(32)    NOT NULL DEFAULT '',
  reference_id    VARCHAR(128)   NOT NULL DEFAULT '',
  affected_amount DECIMAL(36,18) NOT NULL DEFAULT 0,
  -- reverse_batch_id 由 reorgorch 回写；初始为空
  reverse_batch_id VARCHAR(64)   NOT NULL DEFAULT '',
  note            VARCHAR(512)   NOT NULL DEFAULT '',
  created_at      DATETIME(3)    NOT NULL,

  KEY idx_impact_rows_event    (event_id, kind),
  KEY idx_impact_rows_user     (user_id, asset),
  KEY idx_impact_rows_batch    (reverse_batch_id),
  KEY idx_impact_rows_ref      (reference_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='重组影响面明细';

-- ────────────────────────────────────────────────────────────────────────────
-- reverse_batches: 冲正批次表（reorgorch 编排产出）
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reverse_batches (
  id              VARCHAR(64)    NOT NULL PRIMARY KEY,
  event_id        VARCHAR(64)    NOT NULL DEFAULT '',
  -- state: open / planning / executing / committed / aborted
  state           VARCHAR(32)    NOT NULL DEFAULT 'open',
  -- JSON 数组：ImpactRow.ID[]
  impact_row_ids  MEDIUMTEXT     NOT NULL,
  total_legs      INT            NOT NULL DEFAULT 0,
  done_legs       INT            NOT NULL DEFAULT 0,
  failed_legs     INT            NOT NULL DEFAULT 0,
  idem_key        VARCHAR(64)    NOT NULL DEFAULT '',
  note            VARCHAR(512)   NOT NULL DEFAULT '',
  created_at      DATETIME(3)    NOT NULL,
  updated_at      DATETIME(3)    NOT NULL,

  UNIQUE KEY uq_reverse_batches_idem  (idem_key),
  KEY        idx_reverse_batches_event (event_id),
  KEY        idx_reverse_batches_state (state, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='冲正编排批次';

-- ────────────────────────────────────────────────────────────────────────────
-- reverse_legs: 冲正腿表
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reverse_legs (
  id              VARCHAR(64)    NOT NULL PRIMARY KEY,
  batch_id        VARCHAR(64)    NOT NULL DEFAULT '',
  impact_row_id   VARCHAR(64)    NOT NULL DEFAULT '',
  -- kind: debit_reversal / occupancy_release / freeze_rollback / hop_close / incident
  kind            VARCHAR(32)    NOT NULL DEFAULT '',
  execution_order INT            NOT NULL DEFAULT 0,
  -- state: pending / done / skipped / failed / compensated / manual_pending
  state           VARCHAR(32)    NOT NULL DEFAULT 'pending',
  -- idem_key = SHA-256(batch_id|impact_row_id|kind)
  idem_key        VARCHAR(128)   NOT NULL DEFAULT '',
  reference_id    VARCHAR(128)   NOT NULL DEFAULT '',
  amount          DECIMAL(36,18) NOT NULL DEFAULT 0,
  user_id         VARCHAR(64)    NOT NULL DEFAULT '',
  asset           VARCHAR(16)    NOT NULL DEFAULT '',
  high_risk       TINYINT        NOT NULL DEFAULT 0,
  result_note     VARCHAR(512)   NOT NULL DEFAULT '',
  started_at      DATETIME(3)    NULL,
  finished_at     DATETIME(3)    NULL,
  created_at      DATETIME(3)    NOT NULL,

  UNIQUE KEY uq_reverse_legs_idem      (idem_key),
  KEY        idx_reverse_legs_batch     (batch_id, execution_order),
  KEY        idx_reverse_legs_impact    (impact_row_id),
  KEY        idx_reverse_legs_state     (state, batch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='冲正编排腿';

-- ────────────────────────────────────────────────────────────────────────────
-- reverse_guards: 幂等护栏表（防二次冲正）
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reverse_guards (
  id              VARCHAR(64)    NOT NULL PRIMARY KEY,
  impact_row_id   VARCHAR(64)    NOT NULL DEFAULT '',
  leg_kind        VARCHAR(32)    NOT NULL DEFAULT '',
  batch_id        VARCHAR(64)    NOT NULL DEFAULT '',
  leg_id          VARCHAR(64)    NOT NULL DEFAULT '',
  locked_at       DATETIME(3)    NOT NULL,

  -- 唯一约束：同一 (impact_row_id, leg_kind) 只允许一条护栏
  UNIQUE KEY uq_reverse_guards_row_kind (impact_row_id, leg_kind),
  KEY        idx_reverse_guards_batch   (batch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='冲正幂等护栏（防二次冲正）';

-- ────────────────────────────────────────────────────────────────────────────
-- incident_tickets: 事故单表（live-hash / 超时腿 / 失败腿）
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS incident_tickets (
  id              VARCHAR(64)    NOT NULL PRIMARY KEY,
  batch_id        VARCHAR(64)    NOT NULL DEFAULT '',
  leg_id          VARCHAR(64)    NOT NULL DEFAULT '',
  -- kind: live_hash_manual / leg_timeout / leg_failed_manual
  kind            VARCHAR(32)    NOT NULL DEFAULT '',
  description     VARCHAR(1024)  NOT NULL DEFAULT '',
  -- state: open / acknowledged / resolved
  state           VARCHAR(32)    NOT NULL DEFAULT 'open',
  created_at      DATETIME(3)    NOT NULL,
  updated_at      DATETIME(3)    NOT NULL,

  KEY idx_incident_tickets_batch  (batch_id),
  KEY idx_incident_tickets_state  (state, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='重组事故单（live-hash/超时/腿失败）';

-- ────────────────────────────────────────────────────────────────────────────
-- guard_orphan_audit: 孤儿行审计表（GuardScanner 巡检写入）
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS guard_orphan_audit (
  id              BIGINT         NOT NULL AUTO_INCREMENT PRIMARY KEY,
  impact_row_id   VARCHAR(64)    NOT NULL DEFAULT '',
  detected_at     DATETIME(3)    NOT NULL,

  UNIQUE KEY uq_guard_orphan_audit_row (impact_row_id),
  KEY        idx_guard_orphan_detected (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='孤儿ImpactRow审计（未被护栏覆盖）';
