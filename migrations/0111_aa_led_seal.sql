-- 账本快照差分增量校验与静默漂移封堵数据表。
-- 功能：支撑 ledseal 快照调度计划、切片索引、差分批次、漂移行明细、封印根。
-- 边界：只写本模块数据；不修改 ledger_entries / snap_* 等核心账本表。
-- 不变量：aa_led_diff_batches.state 枚举须与 ledseal.BatchState 对齐；
--          aa_led_snap_plans.state 枚举须与 ledseal.PlanState 对齐。
--
-- 作者：TengLong / YuanPei
-- 日期：2022.06.14

-- ─────────────────────────────────────────────────────────
-- 快照调度计划表
-- plan_scope: user_shard | asset_family | full_day
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_led_snap_plans (
  id               VARCHAR(64)    NOT NULL,
  name             VARCHAR(128)   NOT NULL,
  plan_scope       VARCHAR(32)    NOT NULL DEFAULT 'user_shard',
  shard_key        VARCHAR(64)    NOT NULL DEFAULT '',
  asset_family     VARCHAR(32)    NOT NULL DEFAULT '',
  interval_seconds BIGINT         NOT NULL DEFAULT 3600,
  retain_count     INT            NOT NULL DEFAULT 48,
  state            VARCHAR(16)    NOT NULL DEFAULT 'active',
  last_run_at      DATETIME(3)    NULL,
  next_run_at      DATETIME(3)    NULL,
  created_by       VARCHAR(64)    NOT NULL DEFAULT '',
  fence_epoch      BIGINT         NOT NULL DEFAULT 0,
  fence_token      VARCHAR(64)    NOT NULL DEFAULT '',
  created_at       DATETIME(3)    NOT NULL,
  updated_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_aa_led_snap_plans_name (name),
  KEY idx_aa_led_snap_plans_state (state, next_run_at)
);

-- ─────────────────────────────────────────────────────────
-- 快照切片索引表（每次抓拍登记一行）
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_led_snap_slices (
  id               VARCHAR(64)    NOT NULL,
  plan_id          VARCHAR(64)    NOT NULL,
  snap_id          VARCHAR(64)    NOT NULL,
  user_id          VARCHAR(64)    NOT NULL DEFAULT '',
  asset_family     VARCHAR(32)    NOT NULL DEFAULT '',
  gen_seq          BIGINT         NOT NULL DEFAULT 0,
  fence_epoch      BIGINT         NOT NULL DEFAULT 0,
  fence_token      VARCHAR(64)    NOT NULL DEFAULT '',
  checksum         VARCHAR(64)    NOT NULL DEFAULT '',
  position_count   INT            NOT NULL DEFAULT 0,
  state            VARCHAR(16)    NOT NULL DEFAULT 'ready',
  taken_at         DATETIME(3)    NOT NULL,
  created_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_aa_led_snap_slices_plan_seq (plan_id, gen_seq),
  KEY idx_aa_led_snap_slices_snap (snap_id),
  KEY idx_aa_led_snap_slices_user (user_id, plan_id)
);

-- ─────────────────────────────────────────────────────────
-- 差分批次表（两代快照间做一次增量差分，产出漂移行）
-- state: running → sealed | failed
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_led_diff_batches (
  id               VARCHAR(64)    NOT NULL,
  plan_id          VARCHAR(64)    NOT NULL,
  from_slice_id    VARCHAR(64)    NOT NULL,
  to_slice_id      VARCHAR(64)    NOT NULL,
  from_gen_seq     BIGINT         NOT NULL DEFAULT 0,
  to_gen_seq       BIGINT         NOT NULL DEFAULT 0,
  state            VARCHAR(16)    NOT NULL DEFAULT 'running',
  drift_count      INT            NOT NULL DEFAULT 0,
  total_positions  INT            NOT NULL DEFAULT 0,
  max_severity     VARCHAR(16)    NOT NULL DEFAULT 'none',
  seal_root_id     VARCHAR(64)    NOT NULL DEFAULT '',
  err_msg          VARCHAR(512)   NOT NULL DEFAULT '',
  fence_epoch      BIGINT         NOT NULL DEFAULT 0,
  fence_token      VARCHAR(64)    NOT NULL DEFAULT '',
  started_at       DATETIME(3)    NOT NULL,
  sealed_at        DATETIME(3)    NULL,
  created_at       DATETIME(3)    NOT NULL,
  updated_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  KEY idx_aa_led_diff_batches_plan  (plan_id, state, created_at),
  KEY idx_aa_led_diff_batches_slice (from_slice_id, to_slice_id),
  KEY idx_aa_led_diff_batches_state (state, created_at)
);

-- ─────────────────────────────────────────────────────────
-- 漂移行明细表（每个产生差异的 book/asset 位置登记一行）
-- severity: minor | major | critical
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_led_drift_rows (
  id               VARCHAR(64)    NOT NULL,
  batch_id         VARCHAR(64)    NOT NULL,
  plan_id          VARCHAR(64)    NOT NULL,
  user_id          VARCHAR(64)    NOT NULL,
  book             VARCHAR(32)    NOT NULL,
  asset            VARCHAR(32)    NOT NULL,
  avail_before     DECIMAL(36,18) NOT NULL DEFAULT 0,
  avail_after      DECIMAL(36,18) NOT NULL DEFAULT 0,
  frozen_before    DECIMAL(36,18) NOT NULL DEFAULT 0,
  frozen_after     DECIMAL(36,18) NOT NULL DEFAULT 0,
  delta_avail      DECIMAL(36,18) NOT NULL DEFAULT 0,
  delta_frozen     DECIMAL(36,18) NOT NULL DEFAULT 0,
  severity         VARCHAR(16)    NOT NULL DEFAULT 'minor',
  kind             VARCHAR(32)    NOT NULL DEFAULT 'mutation',
  block_write_hint TINYINT(1)     NOT NULL DEFAULT 0,
  confirmed_by     VARCHAR(64)    NOT NULL DEFAULT '',
  confirmed_at     DATETIME(3)    NULL,
  note             VARCHAR(512)   NOT NULL DEFAULT '',
  created_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  KEY idx_aa_led_drift_rows_batch    (batch_id, severity),
  KEY idx_aa_led_drift_rows_user     (user_id, asset, created_at),
  KEY idx_aa_led_drift_rows_severity (severity, created_at)
);

-- ─────────────────────────────────────────────────────────
-- 封印根表（一个 DiffBatch 封印后生成唯一根哈希）
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_led_seal_roots (
  id               VARCHAR(64)    NOT NULL,
  batch_id         VARCHAR(64)    NOT NULL,
  plan_id          VARCHAR(64)    NOT NULL,
  from_gen_seq     BIGINT         NOT NULL DEFAULT 0,
  to_gen_seq       BIGINT         NOT NULL DEFAULT 0,
  root_hash        VARCHAR(64)    NOT NULL,
  drift_count      INT            NOT NULL DEFAULT 0,
  positions_hash   VARCHAR(64)    NOT NULL DEFAULT '',
  meta_json        TEXT           NOT NULL DEFAULT '{}',
  exported_by      VARCHAR(64)    NOT NULL DEFAULT '',
  exported_at      DATETIME(3)    NULL,
  created_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_aa_led_seal_roots_batch (batch_id),
  KEY idx_aa_led_seal_roots_plan  (plan_id, created_at),
  KEY idx_aa_led_seal_roots_hash  (root_hash)
);
