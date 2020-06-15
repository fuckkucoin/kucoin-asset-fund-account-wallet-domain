-- 法币轨道清算轧差断裂与在途挂账引擎数据表。
-- 功能：支撑 fiatbreak 断裂检测、在途挂账、时效老化、双边证明四大能力。
-- 边界：只写 fiat_ 前缀表；不修改 ledger_entries / clearing_* / fiatrail 等现有表。
-- 不变量：
--   fiat_rail_breaks.state 枚举须与 fiatbreak.BreakState 对齐。
--   fiat_suspense.state    枚举须与 fiatbreak.SuspenseState 对齐。
--   bilateral_proofs.proof_hash 追加只写，生成后不可覆盖。
--   表前缀 fiat_；绝不写 clearing_* / ledger_entries / snap_* 核心账本表。
--
-- 作者：ShenYi / LiangFang
-- 日期：2022.03.07

-- ─────────────────────────────────────────────────────────
-- 断裂记录表
-- break_kind: underpay | overpay | duplicate | mid_refund | status_mismatch
-- state:      open → investigating → resolved | written_off
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS fiat_rail_breaks (
  id               VARCHAR(64)     NOT NULL,
  booking_id       VARCHAR(64)     NOT NULL,
  break_kind       VARCHAR(32)     NOT NULL,
  state            VARCHAR(24)     NOT NULL DEFAULT 'open',
  psp_receipt_id   VARCHAR(128)    NOT NULL DEFAULT '',
  psp_amount       DECIMAL(36,18)  NOT NULL DEFAULT 0,
  book_amount      DECIMAL(36,18)  NOT NULL DEFAULT 0,
  delta            DECIMAL(36,18)  NOT NULL DEFAULT 0,
  currency         VARCHAR(8)      NOT NULL DEFAULT 'USD',
  evidence         TEXT            NOT NULL DEFAULT '',
  suspense_id      VARCHAR(64)     NOT NULL DEFAULT '',
  assigned_to      VARCHAR(64)     NOT NULL DEFAULT '',
  resolved_by      VARCHAR(64)     NOT NULL DEFAULT '',
  resolve_note     VARCHAR(512)    NOT NULL DEFAULT '',
  created_at       DATETIME(3)     NOT NULL,
  updated_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_fiat_breaks_booking (booking_id),
  KEY idx_fiat_breaks_state      (state, created_at),
  KEY idx_fiat_breaks_kind       (break_kind, state),
  KEY idx_fiat_breaks_receipt    (psp_receipt_id),
  KEY idx_fiat_breaks_suspense   (suspense_id)
);

-- ─────────────────────────────────────────────────────────
-- 在途挂账表
-- reason: authorized_unsettled | settle_drift | mid_refund | duplicate
-- state:  open → released | consumed
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS fiat_suspense (
  id               VARCHAR(64)     NOT NULL,
  break_id         VARCHAR(64)     NOT NULL DEFAULT '',
  booking_id       VARCHAR(64)     NOT NULL DEFAULT '',
  reason           VARCHAR(32)     NOT NULL,
  state            VARCHAR(16)     NOT NULL DEFAULT 'open',
  asset            VARCHAR(16)     NOT NULL DEFAULT '',
  amount           DECIMAL(36,18)  NOT NULL DEFAULT 0,
  currency         VARCHAR(8)      NOT NULL DEFAULT 'USD',
  sla_deadline     DATETIME(3)     NOT NULL,
  disposed_by      VARCHAR(64)     NOT NULL DEFAULT '',
  disposed_at      DATETIME(3)     NULL,
  disposal_note    VARCHAR(512)    NOT NULL DEFAULT '',
  created_at       DATETIME(3)     NOT NULL,
  updated_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_fiat_suspense_break (break_id),
  KEY idx_fiat_suspense_state    (state, sla_deadline),
  KEY idx_fiat_suspense_booking  (booking_id),
  KEY idx_fiat_suspense_asset    (asset, state),
  KEY idx_fiat_suspense_deadline (sla_deadline, state)
);

-- ─────────────────────────────────────────────────────────
-- 时效老化事件表（追加只写）
-- 每次 SLA 超期触发一次升级，生成一行。
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS fiat_aging_events (
  id               VARCHAR(64)     NOT NULL,
  break_id         VARCHAR(64)     NOT NULL DEFAULT '',
  suspense_id      VARCHAR(64)     NOT NULL DEFAULT '',
  policy_id        VARCHAR(64)     NOT NULL DEFAULT '',
  escalate_desk    VARCHAR(64)     NOT NULL DEFAULT '',
  age_seconds      BIGINT          NOT NULL DEFAULT 0,
  note             VARCHAR(512)    NOT NULL DEFAULT '',
  created_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  KEY idx_fiat_aging_break    (break_id, created_at),
  KEY idx_fiat_aging_suspense (suspense_id, created_at),
  KEY idx_fiat_aging_desk     (escalate_desk, created_at),
  KEY idx_fiat_aging_created  (created_at)
);

-- ─────────────────────────────────────────────────────────
-- 双边证明表（追加只写）
-- proof_hash 生成后不可覆盖；sealed=1 后不得追加新摘要。
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS fiat_bilateral_proofs (
  id               VARCHAR(64)     NOT NULL,
  booking_id       VARCHAR(64)     NOT NULL,
  break_id         VARCHAR(64)     NOT NULL DEFAULT '',
  rail_digest      VARCHAR(64)     NOT NULL,
  ledger_digest    VARCHAR(64)     NOT NULL,
  proof_hash       VARCHAR(64)     NOT NULL,
  sealed           TINYINT(1)      NOT NULL DEFAULT 0,
  sealed_by        VARCHAR(64)     NOT NULL DEFAULT '',
  sealed_at        DATETIME(3)     NULL,
  created_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_fiat_proofs_hash      (proof_hash),
  KEY idx_fiat_proofs_booking  (booking_id, created_at),
  KEY idx_fiat_proofs_break    (break_id, created_at),
  KEY idx_fiat_proofs_sealed   (sealed, created_at)
);

-- ─────────────────────────────────────────────────────────
-- SLA 策略配置表（可选持久化；默认由 Go 代码内存装载）
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS fiat_aging_policies (
  id                  VARCHAR(64)    NOT NULL,
  break_kind          VARCHAR(32)    NOT NULL DEFAULT '',
  suspense_reason     VARCHAR(32)    NOT NULL DEFAULT '',
  max_age_sec         BIGINT         NOT NULL DEFAULT 259200,
  escalate_desk       VARCHAR(64)    NOT NULL DEFAULT 'break_desk',
  auto_write_off      TINYINT(1)     NOT NULL DEFAULT 0,
  write_off_threshold DECIMAL(20,8)  NOT NULL DEFAULT 0,
  enabled             TINYINT(1)     NOT NULL DEFAULT 1,
  created_at          DATETIME(3)    NOT NULL,
  updated_at          DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  KEY idx_fiat_policies_kind    (break_kind, enabled),
  KEY idx_fiat_policies_reason  (suspense_reason, enabled)
);
