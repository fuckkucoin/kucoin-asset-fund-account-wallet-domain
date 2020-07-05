-- 迁移 0053：prefroute 偏好路由漂移封堵表集
-- 涵盖：fp_pref_generations, fp_default_account_prefs, fp_pending_switches,
--       fp_pref_leases, fp_route_snaps, fp_pref_drift_cases,
--       fp_drift_proofs, fp_inflight_credits
-- 作者：Marlin  日期：2020-12-01

CREATE TABLE IF NOT EXISTS fp_pref_generations (
  id           VARCHAR(64)   NOT NULL PRIMARY KEY,
  user_id      VARCHAR(64)   NOT NULL,
  generation   BIGINT        NOT NULL DEFAULT 0,
  default_book VARCHAR(32)   NOT NULL DEFAULT 'MAIN',
  bumped_by    VARCHAR(64)   NOT NULL DEFAULT '',
  source_req_id VARCHAR(128) NOT NULL DEFAULT '',
  bumped_at    DATETIME(3)   NOT NULL,
  UNIQUE KEY uk_fp_pref_gen_user_gen (user_id, generation),
  KEY idx_fp_pref_gen_user (user_id, bumped_at)
);

CREATE TABLE IF NOT EXISTS fp_default_account_prefs (
  user_id    VARCHAR(64)  NOT NULL PRIMARY KEY,
  book       VARCHAR(32)  NOT NULL DEFAULT 'MAIN',
  version    BIGINT       NOT NULL DEFAULT 0,
  updated_at DATETIME(3)  NOT NULL,
  updated_by VARCHAR(64)  NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS fp_pending_switches (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id      VARCHAR(64)  NOT NULL,
  from_book    VARCHAR(32)  NOT NULL DEFAULT '',
  to_book      VARCHAR(32)  NOT NULL DEFAULT '',
  state        VARCHAR(16)  NOT NULL DEFAULT 'queued',
  requested_by VARCHAR(64)  NOT NULL DEFAULT '',
  requested_at DATETIME(3)  NOT NULL,
  applied_at   DATETIME(3)  NULL,
  cancelled_at DATETIME(3)  NULL,
  cancel_note  VARCHAR(255) NOT NULL DEFAULT '',
  fence_epoch  BIGINT       NOT NULL DEFAULT 0,
  fence_token  VARCHAR(64)  NOT NULL DEFAULT '',
  KEY idx_fp_pending_sw_user_state (user_id, state, requested_at)
);

-- fp_pref_leases：偏好租约；绑定贷记路由到偏好世代快照
CREATE TABLE IF NOT EXISTS fp_pref_leases (
  id            VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id       VARCHAR(64)  NOT NULL,
  generation    BIGINT       NOT NULL DEFAULT 0,
  default_book  VARCHAR(32)  NOT NULL DEFAULT 'MAIN',
  state         VARCHAR(16)  NOT NULL DEFAULT 'open',
  route_snap_id VARCHAR(64)  NOT NULL DEFAULT '',
  credit_txn_id VARCHAR(128) NOT NULL DEFAULT '',
  issued_at     DATETIME(3)  NOT NULL,
  expires_at    DATETIME(3)  NOT NULL,
  bound_at      DATETIME(3)  NULL,
  switched_at   DATETIME(3)  NULL,
  blocked_at    DATETIME(3)  NULL,
  block_reason  VARCHAR(255) NOT NULL DEFAULT '',
  issued_by     VARCHAR(64)  NOT NULL DEFAULT '',
  fence_epoch   BIGINT       NOT NULL DEFAULT 0,
  fence_token   VARCHAR(64)  NOT NULL DEFAULT '',
  KEY idx_fp_pref_leases_user_state (user_id, state, issued_at),
  KEY idx_fp_pref_leases_txn (credit_txn_id)
);

-- fp_route_snaps：路由快照；记录贷记绑定时的科目与世代
CREATE TABLE IF NOT EXISTS fp_route_snaps (
  id            VARCHAR(64)   NOT NULL PRIMARY KEY,
  lease_id      VARCHAR(64)   NOT NULL,
  user_id       VARCHAR(64)   NOT NULL,
  generation    BIGINT        NOT NULL DEFAULT 0,
  bound_book    VARCHAR(32)   NOT NULL DEFAULT '',
  credited_book VARCHAR(32)   NOT NULL DEFAULT '',
  credit_txn_id VARCHAR(128)  NOT NULL DEFAULT '',
  asset         VARCHAR(16)   NOT NULL DEFAULT '',
  amount        DECIMAL(36,18) NOT NULL DEFAULT 0,
  snap_at       DATETIME(3)   NOT NULL,
  credited_at   DATETIME(3)   NULL,
  drift_case_id VARCHAR(64)   NOT NULL DEFAULT '',
  KEY idx_fp_route_snaps_lease (lease_id),
  KEY idx_fp_route_snaps_user (user_id, snap_at),
  KEY idx_fp_route_snaps_txn (credit_txn_id)
);

-- fp_pref_drift_cases：路由漂移案例；贷记科目 ≠ 快照科目时产出
CREATE TABLE IF NOT EXISTS fp_pref_drift_cases (
  id            VARCHAR(64)   NOT NULL PRIMARY KEY,
  user_id       VARCHAR(64)   NOT NULL,
  lease_id      VARCHAR(64)   NOT NULL DEFAULT '',
  route_snap_id VARCHAR(64)   NOT NULL DEFAULT '',
  bound_book    VARCHAR(32)   NOT NULL DEFAULT '',
  credited_book VARCHAR(32)   NOT NULL DEFAULT '',
  generation    BIGINT        NOT NULL DEFAULT 0,
  credit_txn_id VARCHAR(128)  NOT NULL DEFAULT '',
  asset         VARCHAR(16)   NOT NULL DEFAULT '',
  amount        DECIMAL(36,18) NOT NULL DEFAULT 0,
  state         VARCHAR(16)   NOT NULL DEFAULT 'open',
  detected_at   DATETIME(3)   NOT NULL,
  proven_at     DATETIME(3)   NULL,
  closed_at     DATETIME(3)   NULL,
  proof_id      VARCHAR(64)   NOT NULL DEFAULT '',
  proof_summary VARCHAR(512)  NOT NULL DEFAULT '',
  closed_by     VARCHAR(64)   NOT NULL DEFAULT '',
  KEY idx_fp_pref_drift_user_state (user_id, state, detected_at),
  KEY idx_fp_pref_drift_lease (lease_id),
  KEY idx_fp_pref_drift_snap (route_snap_id)
);

-- fp_drift_proofs：漂移证明；关联 PrefDriftCase
CREATE TABLE IF NOT EXISTS fp_drift_proofs (
  id           VARCHAR(64)   NOT NULL PRIMARY KEY,
  drift_case_id VARCHAR(64)  NOT NULL,
  submitted_by VARCHAR(64)   NOT NULL DEFAULT '',
  summary      VARCHAR(512)  NOT NULL DEFAULT '',
  evidence     TEXT          NOT NULL,
  submitted_at DATETIME(3)   NOT NULL,
  confirmed_by VARCHAR(64)   NOT NULL DEFAULT '',
  confirmed_at DATETIME(3)   NULL,
  KEY idx_fp_drift_proofs_case (drift_case_id, submitted_at)
);

-- fp_inflight_credits：在途贷记追踪；用于切换互斥检查
CREATE TABLE IF NOT EXISTS fp_inflight_credits (
  id            VARCHAR(64)   NOT NULL PRIMARY KEY,
  user_id       VARCHAR(64)   NOT NULL,
  txn_id        VARCHAR(128)  NOT NULL,
  asset         VARCHAR(16)   NOT NULL DEFAULT '',
  amount        DECIMAL(36,18) NOT NULL DEFAULT 0,
  state         VARCHAR(16)   NOT NULL DEFAULT 'pending',
  lease_id      VARCHAR(64)   NOT NULL DEFAULT '',
  route_snap_id VARCHAR(64)   NOT NULL DEFAULT '',
  created_at    DATETIME(3)   NOT NULL,
  updated_at    DATETIME(3)   NOT NULL,
  UNIQUE KEY uk_fp_inflight_txn (txn_id),
  KEY idx_fp_inflight_user_state (user_id, state, created_at)
);
