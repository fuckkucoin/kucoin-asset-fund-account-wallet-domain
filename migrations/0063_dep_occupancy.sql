-- 功能：充值风险档占用的策略、批次、档位、信号、桶和审计。
-- 边界：这些表不写账本分录，不替代延迟入账或提前入账回滚。
-- 不变量：同一 transfer 只占一行；用户资产桶与批次占用额必须能对上。
-- 作者：Axel
CREATE TABLE IF NOT EXISTS dep_occupancy_policy (
  id VARCHAR(96) NOT NULL PRIMARY KEY,
  kyc VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  source VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  ratio_bps INT NOT NULL,
  max_window_sec INT NOT NULL,
  min_window_sec INT NOT NULL DEFAULT 0,
  tier_floor VARCHAR(8) NOT NULL DEFAULT 'T0',
  version INT NOT NULL,
  priority INT NOT NULL DEFAULT 0,
  enabled TINYINT NOT NULL DEFAULT 1,
  note VARCHAR(255) NOT NULL DEFAULT '',
  UNIQUE KEY uk_dep_occ_pol (kyc, asset, source, chain, version)
);

CREATE TABLE IF NOT EXISTS dep_occupancy_lot (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  transfer_id VARCHAR(64) NOT NULL,
  idem VARCHAR(128) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  source VARCHAR(16) NOT NULL,
  kyc VARCHAR(16) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  from_addr VARCHAR(128) NOT NULL DEFAULT '',
  observed DECIMAL(36,18) NOT NULL,
  occupied DECIMAL(36,18) NOT NULL,
  free_amt DECIMAL(36,18) NOT NULL,
  state VARCHAR(32) NOT NULL,
  tier VARCHAR(8) NOT NULL,
  policy_id VARCHAR(96) NOT NULL,
  ratio_bps INT NOT NULL,
  max_window_sec INT NOT NULL,
  min_window_sec INT NOT NULL DEFAULT 0,
  early_credit TINYINT NOT NULL DEFAULT 0,
  confirms INT NOT NULL DEFAULT 0,
  full_need INT NOT NULL DEFAULT 0,
  observed_at DATETIME(3) NOT NULL,
  release_at DATETIME(3) NULL,
  deadline DATETIME(3) NOT NULL,
  cause VARCHAR(64) NOT NULL DEFAULT '',
  window_hit TINYINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_dep_occ_xfer (transfer_id),
  KEY idx_dep_occ_user (user_id, asset, state)
);

CREATE TABLE IF NOT EXISTS dep_occupancy_bucket (
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  in_transit DECIMAL(36,18) NOT NULL,
  available DECIMAL(36,18) NOT NULL,
  rolled DECIMAL(36,18) NOT NULL,
  PRIMARY KEY (user_id, asset)
);

CREATE TABLE IF NOT EXISTS dep_occupancy_tier (
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  tier VARCHAR(8) NOT NULL,
  score INT NOT NULL,
  clean_run INT NOT NULL DEFAULT 0,
  kyc VARCHAR(16) NOT NULL DEFAULT '',
  last_source VARCHAR(16) NOT NULL DEFAULT '',
  since_at DATETIME(3) NOT NULL,
  last_up DATETIME(3) NULL,
  last_down DATETIME(3) NULL,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  version INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS dep_occupancy_signal (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  weight INT NOT NULL,
  ref VARCHAR(128) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL DEFAULT '',
  detail VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_dep_occ_sig_user (user_id, at)
);

CREATE TABLE IF NOT EXISTS dep_occupancy_audit (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  lot_id VARCHAR(96) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  event VARCHAR(32) NOT NULL,
  from_state VARCHAR(32) NOT NULL DEFAULT '',
  to_state VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL DEFAULT '0',
  actor VARCHAR(64) NOT NULL,
  at DATETIME(3) NOT NULL,
  KEY idx_dep_occ_audit_lot (lot_id, at)
);
