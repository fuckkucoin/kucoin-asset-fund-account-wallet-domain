-- 账户资产：审查案、占用、绑址、审批、充提唯一键、围栏。
-- 金额 DECIMAL(36,18)。(chain, tx_hash) 唯一。

-- 作者：Kinsley
CREATE TABLE IF NOT EXISTS aa_fences (
  row_id VARCHAR(191) NOT NULL PRIMARY KEY,
  epoch BIGINT NOT NULL DEFAULT 0,
  token VARCHAR(64) NOT NULL,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_cases (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  movement_id VARCHAR(128) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  reason VARCHAR(255) NOT NULL,
  state VARCHAR(32) NOT NULL,
  assignee VARCHAR(64) NOT NULL DEFAULT '',
  decided_by VARCHAR(64) NOT NULL DEFAULT '',
  policy_ver INT NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_cases_move (movement_id),
  KEY idx_aa_cases_state (state, updated_at)
);

CREATE TABLE IF NOT EXISTS aa_case_audit (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  case_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  event VARCHAR(32) NOT NULL,
  from_state VARCHAR(32) NOT NULL DEFAULT '',
  to_state VARCHAR(32) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_case_audit (case_id, at)
);

CREATE TABLE IF NOT EXISTS aa_usage_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(16) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL,
  idem VARCHAR(128) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_usage_idem (idem),
  KEY idx_aa_usage_user (kind, user_id, asset, at)
);

CREATE TABLE IF NOT EXISTS aa_addr_binds (
  bind_key VARCHAR(191) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  family VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  allocated TINYINT NOT NULL DEFAULT 1,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  bound_at DATETIME(3) NOT NULL,
  reuse_after DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_addr (address)
);

CREATE TABLE IF NOT EXISTS aa_approvals (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(16) NOT NULL,
  movement_id VARCHAR(128) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL DEFAULT '',
  dest VARCHAR(128) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL DEFAULT 0,
  requested_by VARCHAR(64) NOT NULL,
  approved_by VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_deposits (
  chain VARCHAR(16) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (chain, tx_hash)
);

CREATE TABLE IF NOT EXISTS aa_withdraws (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  idem VARCHAR(128) NOT NULL,
  state VARCHAR(32) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_wd_idem (idem)
);

CREATE TABLE IF NOT EXISTS aa_kv (
  row_id VARCHAR(191) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  row_key VARCHAR(191) NOT NULL,
  body BLOB,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  UNIQUE KEY uk_aa_kv_kind_key (kind, row_key)
);
