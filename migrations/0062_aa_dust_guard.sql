-- 粉尘攻击防护：信号、评分、隔离、轮换计划、策略、挡贷记工单、审计。
-- 表前缀 dust_ / addrpool_guard_ 避免与 r1/r2 表冲突。
-- 不改对账三方公式，不改 dep: 速度键，不改归因 Resolve 语义。
-- 作者：Axel

CREATE TABLE IF NOT EXISTS dust_signal (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  from_addr VARCHAR(128) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL DEFAULT 0,
  tx_hash VARCHAR(128) NOT NULL DEFAULT '',
  signal_kind VARCHAR(32) NOT NULL DEFAULT '',
  seen_at DATETIME(3) NOT NULL,
  KEY idx_dust_sig_addr (chain, address, seen_at),
  KEY idx_dust_sig_from (chain, from_addr, seen_at),
  KEY idx_dust_sig_hash (tx_hash)
);

CREATE TABLE IF NOT EXISTS dust_attack_score (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  family VARCHAR(16) NOT NULL DEFAULT '',
  score INT NOT NULL DEFAULT 0,
  level VARCHAR(16) NOT NULL DEFAULT 'none',
  flood_count INT NOT NULL DEFAULT 0,
  cluster_n INT NOT NULL DEFAULT 0,
  surface_hit TINYINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_dust_score_addr (chain, address)
);

CREATE TABLE IF NOT EXISTS addrpool_guard_quarantine (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  family VARCHAR(16) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  trigger_score INT NOT NULL DEFAULT 0,
  trigger_level VARCHAR(16) NOT NULL DEFAULT '',
  trigger_reason VARCHAR(255) NOT NULL DEFAULT '',
  forensics_until DATETIME(3) NOT NULL,
  opened_by VARCHAR(64) NOT NULL DEFAULT '',
  closed_by VARCHAR(64) NOT NULL DEFAULT '',
  rotate_plan_id VARCHAR(64) NOT NULL DEFAULT '',
  event_fact_id VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_agq_addr (chain, address),
  KEY idx_agq_state (state, updated_at)
);

CREATE TABLE IF NOT EXISTS addrpool_guard_rotate_plan (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  old_address VARCHAR(128) NOT NULL,
  new_address VARCHAR(128) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  case_id VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  trigger_kind VARCHAR(32) NOT NULL DEFAULT '',
  approval_id VARCHAR(64) NOT NULL DEFAULT '',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  executed_by VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  sealed_at DATETIME(3),
  KEY idx_agrp_case (case_id),
  KEY idx_agrp_state (state),
  KEY idx_agrp_addr (chain, old_address)
);

CREATE TABLE IF NOT EXISTS addrpool_guard_policy (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  version INT NOT NULL DEFAULT 1,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  flood_threshold INT NOT NULL DEFAULT 50,
  flood_window_sec INT NOT NULL DEFAULT 3600,
  cluster_n INT NOT NULL DEFAULT 10,
  score_attack INT NOT NULL DEFAULT 75,
  score_suspect INT NOT NULL DEFAULT 40,
  water_level INT NOT NULL DEFAULT 20,
  forensics_sec INT NOT NULL DEFAULT 86400,
  auto_quarantine TINYINT NOT NULL DEFAULT 1,
  auto_rotate TINYINT NOT NULL DEFAULT 0,
  active TINYINT NOT NULL DEFAULT 1,
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_by VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_agpol_chain (chain, active, version)
);

CREATE TABLE IF NOT EXISTS addrpool_guard_ticket (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  tx_hash VARCHAR(128) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL DEFAULT 0,
  case_id VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_agt_addr (chain, address),
  KEY idx_agt_state (state, updated_at),
  KEY idx_agt_hash (tx_hash)
);

CREATE TABLE IF NOT EXISTS addrpool_guard_audit (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  ref_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  event VARCHAR(32) NOT NULL,
  from_state VARCHAR(32) NOT NULL DEFAULT '',
  to_state VARCHAR(32) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  fact_id VARCHAR(64) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aga_ref (kind, ref_id, at),
  KEY idx_aga_at (at)
);
