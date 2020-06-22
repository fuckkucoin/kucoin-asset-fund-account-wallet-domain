-- 守恒闸快照世代租约与部分批次失衡封堵持久化表。
-- 模块：internal/guardlease；任务 D · 第十一轮。
--
-- 作者：Axel
-- 表清单：
--   fg_guard_generations   — user×asset 世代计数器
--   fg_snap_leases         — 快照世代租约
--   fg_fence_requests      — 租约窗口内活账写排队记录
--   fg_partial_imbalances  — 部分批次金额失衡证明
--   fg_conserve_proofs     — 守恒前后 Bundle 证明

CREATE TABLE IF NOT EXISTS fg_guard_generations (
  user_id     VARCHAR(64)  NOT NULL,
  asset       VARCHAR(16)  NOT NULL,
  generation  BIGINT       NOT NULL DEFAULT 0,
  updated_at  DATETIME(3)  NOT NULL,
  PRIMARY KEY (user_id, asset)
);

CREATE TABLE IF NOT EXISTS fg_snap_leases (
  lease_id      VARCHAR(64)  NOT NULL PRIMARY KEY,
  generation    BIGINT       NOT NULL,
  user_id       VARCHAR(64)  NOT NULL,
  asset         VARCHAR(16)  NOT NULL,
  snapshot_hash VARCHAR(64)  NOT NULL,
  state         VARCHAR(16)  NOT NULL DEFAULT 'open',
  issued_at     DATETIME(3)  NOT NULL,
  expires_at    DATETIME(3)  NOT NULL,
  bound_at      DATETIME(3)  NULL,
  consumed_at   DATETIME(3)  NULL,
  actor_id      VARCHAR(64)  NOT NULL,
  KEY idx_fg_leases_user_asset (user_id, asset, state),
  KEY idx_fg_leases_expires   (state, expires_at)
);

CREATE TABLE IF NOT EXISTS fg_fence_requests (
  fence_id    VARCHAR(64)   NOT NULL PRIMARY KEY,
  lease_id    VARCHAR(64)   NOT NULL,
  user_id     VARCHAR(64)   NOT NULL,
  asset       VARCHAR(16)   NOT NULL,
  kind        VARCHAR(32)   NOT NULL DEFAULT '',
  amount      VARCHAR(64)   NOT NULL DEFAULT '0',
  queued_at   DATETIME(3)   NOT NULL,
  cleared_at  DATETIME(3)   NULL,
  KEY idx_fg_fence_lease (lease_id, cleared_at),
  KEY idx_fg_fence_user  (user_id, asset, cleared_at)
);

CREATE TABLE IF NOT EXISTS fg_partial_imbalances (
  imbalance_id   VARCHAR(64)   NOT NULL PRIMARY KEY,
  batch_id       VARCHAR(64)   NOT NULL,
  user_id        VARCHAR(64)   NOT NULL,
  asset          VARCHAR(16)   NOT NULL,
  expected_total VARCHAR(64)   NOT NULL,
  actual_total   VARCHAR(64)   NOT NULL,
  delta          VARCHAR(64)   NOT NULL,
  state          VARCHAR(16)   NOT NULL DEFAULT 'open',
  detected_at    DATETIME(3)   NOT NULL,
  proven_at      DATETIME(3)   NULL,
  closed_at      DATETIME(3)   NULL,
  actor1         VARCHAR(64)   NULL,
  actor2         VARCHAR(64)   NULL,
  KEY idx_fg_imbalances_state (state, detected_at),
  KEY idx_fg_imbalances_user  (user_id, asset)
);

CREATE TABLE IF NOT EXISTS fg_conserve_proofs (
  proof_id      VARCHAR(64)   NOT NULL PRIMARY KEY,
  lease_id      VARCHAR(64)   NOT NULL DEFAULT '',
  user_id       VARCHAR(64)   NOT NULL,
  asset         VARCHAR(16)   NOT NULL,
  before_total  VARCHAR(64)   NOT NULL,
  after_total   VARCHAR(64)   NOT NULL,
  snap_hash     VARCHAR(64)   NOT NULL DEFAULT '',
  state         VARCHAR(16)   NOT NULL DEFAULT 'pending',
  created_at    DATETIME(3)   NOT NULL,
  resolved_at   DATETIME(3)   NULL,
  fail_reason   TEXT          NULL,
  cleared_by    VARCHAR(128)  NULL,
  escalated_at  DATETIME(3)   NULL,
  KEY idx_fg_proofs_state   (state, created_at),
  KEY idx_fg_proofs_user    (user_id, asset),
  KEY idx_fg_proofs_lease   (lease_id)
);
