-- 多科目账本账单组装活账漂移封堵引擎 DDL
-- 作者：Kinsley
-- 日期：2021.11.22

-- 快照租约表
CREATE TABLE IF NOT EXISTS bookbill_snap_leases (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id         VARCHAR(64)  NOT NULL,
  asset           VARCHAR(32)  NOT NULL,
  snap_window     VARCHAR(256) NOT NULL DEFAULT '',
  state           VARCHAR(16)  NOT NULL DEFAULT 'pinned',
  snap_gen        BIGINT       NOT NULL DEFAULT 1,
  book_snaps      JSON         NOT NULL,
  pin_ttl_ms      BIGINT       NOT NULL DEFAULT 600000,
  pinned_at       DATETIME(3)  NOT NULL,
  expires_at      DATETIME(3)  NOT NULL,
  assemble_start  DATETIME(3)  NULL,
  sealed_at       DATETIME(3)  NULL,
  drifted_at      DATETIME(3)  NULL,
  aborted_at      DATETIME(3)  NULL,
  repin_count     INT          NOT NULL DEFAULT 0,
  max_repin       INT          NOT NULL DEFAULT 3,
  sealed_result   JSON         NULL,
  drift_proof_id  VARCHAR(64)  NOT NULL DEFAULT '',
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_bsl_user ON bookbill_snap_leases (user_id);
CREATE INDEX idx_bsl_asset ON bookbill_snap_leases (asset);
CREATE INDEX idx_bsl_state ON bookbill_snap_leases (state);
CREATE INDEX idx_bsl_expires ON bookbill_snap_leases (expires_at);
CREATE INDEX idx_bsl_window ON bookbill_snap_leases (user_id, asset, snap_window);
CREATE INDEX idx_bsl_user_asset ON bookbill_snap_leases (user_id, asset, state);

-- 漂移证明表
CREATE TABLE IF NOT EXISTS bookbill_snap_drift_proofs (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  lease_id        VARCHAR(64)  NOT NULL,
  user_id         VARCHAR(64)  NOT NULL,
  asset           VARCHAR(32)  NOT NULL,
  snap_gen        BIGINT       NOT NULL DEFAULT 0,
  kind            VARCHAR(32)  NOT NULL,
  book            VARCHAR(32)  NOT NULL DEFAULT '',
  old_waterline   VARCHAR(128) NOT NULL DEFAULT '',
  new_waterline   VARCHAR(128) NOT NULL DEFAULT '',
  old_gen         BIGINT       NOT NULL DEFAULT 0,
  new_gen         BIGINT       NOT NULL DEFAULT 0,
  debit_before    VARCHAR(64)  NOT NULL DEFAULT '',
  credit_before   VARCHAR(64)  NOT NULL DEFAULT '',
  debit_after     VARCHAR(64)  NOT NULL DEFAULT '',
  credit_after    VARCHAR(64)  NOT NULL DEFAULT '',
  delta           VARCHAR(64)  NOT NULL DEFAULT '',
  detail          TEXT         NOT NULL DEFAULT '',
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_bsdp_lease ON bookbill_snap_drift_proofs (lease_id);
CREATE INDEX idx_bsdp_user ON bookbill_snap_drift_proofs (user_id);
CREATE INDEX idx_bsdp_kind ON bookbill_snap_drift_proofs (kind);
CREATE INDEX idx_bsdp_asset ON bookbill_snap_drift_proofs (asset);
