-- 安全变更冷却标记与出金预检窗口竞态收口引擎 DDL
-- 作者：Kinsley
-- 日期：2021.08.17

-- 冷却代次租约表
CREATE TABLE IF NOT EXISTS seclock_cool_leases (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id         VARCHAR(64)  NOT NULL,
  reason          VARCHAR(64)  NOT NULL,
  cool_gen        BIGINT       NOT NULL DEFAULT 1,
  state           VARCHAR(16)  NOT NULL DEFAULT 'open',
  bound_to        VARCHAR(64)  NOT NULL DEFAULT '',
  lease_expiry    DATETIME(3)  NOT NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  bound_at        DATETIME(3)  NULL,
  expired_at      DATETIME(3)  NULL
);

CREATE INDEX idx_seclock_cl_user ON seclock_cool_leases (user_id);
CREATE INDEX idx_seclock_cl_state ON seclock_cool_leases (state);
CREATE INDEX idx_seclock_cl_expiry ON seclock_cool_leases (lease_expiry);

-- 预检绑定令牌表
CREATE TABLE IF NOT EXISTS seclock_preflight_binds (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id         VARCHAR(64)  NOT NULL,
  withdraw_id     VARCHAR(64)  NOT NULL,
  cool_gen        BIGINT       NOT NULL DEFAULT 0,
  lease_id        VARCHAR(64)  NOT NULL DEFAULT '',
  state           VARCHAR(16)  NOT NULL DEFAULT 'open',
  token           VARCHAR(64)  NOT NULL,
  checked_at      DATETIME(3)  NOT NULL,
  bind_expiry     DATETIME(3)  NOT NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  revoked_at      DATETIME(3)  NULL,
  consumed_at     DATETIME(3)  NULL,
  revoke_reason   VARCHAR(256) NOT NULL DEFAULT '',
  UNIQUE KEY uk_spb_withdraw (withdraw_id)
);

CREATE INDEX idx_seclock_pb_user ON seclock_preflight_binds (user_id);
CREATE INDEX idx_seclock_pb_state ON seclock_preflight_binds (state);
CREATE INDEX idx_seclock_pb_token ON seclock_preflight_binds (token);
CREATE INDEX idx_seclock_pb_expiry ON seclock_preflight_binds (bind_expiry);
CREATE INDEX idx_seclock_pb_lease ON seclock_preflight_binds (lease_id);

-- 窗口泄漏案例表
CREATE TABLE IF NOT EXISTS seclock_window_cases (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id         VARCHAR(64)  NOT NULL,
  withdraw_id     VARCHAR(64)  NOT NULL,
  bind_id         VARCHAR(64)  NOT NULL DEFAULT '',
  lease_id        VARCHAR(64)  NOT NULL DEFAULT '',
  kind            VARCHAR(32)  NOT NULL,
  state           VARCHAR(16)  NOT NULL DEFAULT 'open',
  detail          TEXT         NOT NULL,
  proof_ids       TEXT         NOT NULL DEFAULT '',
  resolution      TEXT         NOT NULL DEFAULT '',
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  proven_at       DATETIME(3)  NULL,
  closed_at       DATETIME(3)  NULL
);

CREATE INDEX idx_seclock_wc_user ON seclock_window_cases (user_id);
CREATE INDEX idx_seclock_wc_withdraw ON seclock_window_cases (withdraw_id);
CREATE INDEX idx_seclock_wc_state ON seclock_window_cases (state);

-- 窗口泄漏证明表
CREATE TABLE IF NOT EXISTS seclock_window_proofs (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  case_id         VARCHAR(64)  NOT NULL,
  user_id         VARCHAR(64)  NOT NULL,
  withdraw_id     VARCHAR(64)  NOT NULL,
  lease_id        VARCHAR(64)  NOT NULL DEFAULT '',
  bind_id         VARCHAR(64)  NOT NULL DEFAULT '',
  mark_reason     VARCHAR(64)  NOT NULL,
  preflight_gen   BIGINT       NOT NULL DEFAULT 0,
  mark_gen        BIGINT       NOT NULL DEFAULT 0,
  preflight_time  DATETIME(3)  NOT NULL,
  mark_time       DATETIME(3)  NOT NULL,
  ledger_lock_until DATETIME(3) NULL,
  sealed          TINYINT      NOT NULL DEFAULT 0,
  sealed_at       DATETIME(3)  NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_seclock_wp_case ON seclock_window_proofs (case_id);
CREATE INDEX idx_seclock_wp_user ON seclock_window_proofs (user_id);
CREATE INDEX idx_seclock_wp_withdraw ON seclock_window_proofs (withdraw_id);
