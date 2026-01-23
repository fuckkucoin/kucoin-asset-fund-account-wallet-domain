-- 区级托管库存与跨区再平衡。证明链仍在 aa_sweep_proof_*，本脚本不改那些表。
-- 水位、隔离策略、计划与腿只服务库存防火墙；不写热预留金科目。

-- 作者：Axel
CREATE TABLE IF NOT EXISTS aa_regio_regions (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  code VARCHAR(32) NOT NULL,
  catalog_code VARCHAR(16) NOT NULL,
  name VARCHAR(128) NOT NULL,
  role VARCHAR(16) NOT NULL,
  firewall VARCHAR(16) NOT NULL,
  firewall_reason VARCHAR(255) NOT NULL DEFAULT '',
  halted TINYINT NOT NULL DEFAULT 0,
  halt_actor VARCHAR(64) NOT NULL DEFAULT '',
  halt_reason VARCHAR(255) NOT NULL DEFAULT '',
  halt_at VARCHAR(32) NOT NULL DEFAULT '',
  ready TINYINT NOT NULL DEFAULT 1,
  ready_detail VARCHAR(255) NOT NULL DEFAULT '',
  created_at VARCHAR(32) NOT NULL,
  updated_at VARCHAR(32) NOT NULL,
  UNIQUE KEY uk_aa_regio_code (code)
);

CREATE TABLE IF NOT EXISTS aa_regio_inv (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  region_id VARCHAR(64) NOT NULL,
  tier VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  available DECIMAL(36,18) NOT NULL,
  frozen DECIMAL(36,18) NOT NULL,
  observed DECIMAL(36,18) NOT NULL,
  seq BIGINT NOT NULL,
  source VARCHAR(64) NOT NULL DEFAULT '',
  observed_at VARCHAR(32) NOT NULL,
  updated_at VARCHAR(32) NOT NULL,
  UNIQUE KEY uk_aa_regio_inv (region_id, tier, asset, chain)
);

CREATE TABLE IF NOT EXISTS aa_regio_watermarks (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  region_id VARCHAR(64) NOT NULL,
  tier VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  low_amt DECIMAL(36,18) NOT NULL,
  target_amt DECIMAL(36,18) NOT NULL,
  high_amt DECIMAL(36,18) NOT NULL,
  min_leg DECIMAL(36,18) NOT NULL DEFAULT 0,
  max_leg DECIMAL(36,18) NOT NULL DEFAULT 0,
  daily_cap DECIMAL(36,18) NOT NULL DEFAULT 0,
  used_amt DECIMAL(36,18) NOT NULL DEFAULT 0,
  used_day VARCHAR(16) NOT NULL DEFAULT '',
  window_start INT NOT NULL DEFAULT 0,
  window_end INT NOT NULL DEFAULT 0,
  max_legs INT NOT NULL DEFAULT 8,
  enabled TINYINT NOT NULL DEFAULT 1,
  actor VARCHAR(64) NOT NULL,
  updated_at VARCHAR(32) NOT NULL,
  UNIQUE KEY uk_aa_regio_wm (region_id, tier, asset, chain)
);

CREATE TABLE IF NOT EXISTS aa_regio_iso_policies (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  region_id VARCHAR(64) NOT NULL,
  block_cross_hot TINYINT NOT NULL DEFAULT 1,
  block_wrong_rebal TINYINT NOT NULL DEFAULT 1,
  drift_bps INT NOT NULL DEFAULT 0,
  fail_to VARCHAR(16) NOT NULL DEFAULT 'blocked',
  actor VARCHAR(64) NOT NULL,
  updated_at VARCHAR(32) NOT NULL,
  UNIQUE KEY uk_aa_regio_iso (region_id)
);

CREATE TABLE IF NOT EXISTS aa_regio_obs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  region_id VARCHAR(64) NOT NULL,
  tier VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  source VARCHAR(64) NOT NULL,
  available DECIMAL(36,18) NOT NULL,
  frozen DECIMAL(36,18) NOT NULL,
  seq BIGINT NOT NULL,
  observed_at VARCHAR(32) NOT NULL,
  UNIQUE KEY uk_aa_regio_obs (region_id, tier, asset, chain, source)
);

CREATE TABLE IF NOT EXISTS aa_georebal_plans (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  idem VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  tier VARCHAR(16) NOT NULL,
  src_region VARCHAR(64) NOT NULL,
  dst_region VARCHAR(64) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  state VARCHAR(16) NOT NULL,
  approval VARCHAR(16) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  code VARCHAR(64) NOT NULL DEFAULT '',
  hooks_json TEXT NOT NULL,
  created_at VARCHAR(32) NOT NULL,
  updated_at VARCHAR(32) NOT NULL,
  UNIQUE KEY uk_aa_georebal_idem (idem),
  KEY idx_aa_georebal_state (state, updated_at)
);

CREATE TABLE IF NOT EXISTS aa_georebal_legs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  src_region VARCHAR(64) NOT NULL,
  dst_region VARCHAR(64) NOT NULL,
  tier VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL,
  state VARCHAR(16) NOT NULL,
  sweep_intent_id VARCHAR(64) NOT NULL DEFAULT '',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  created_at VARCHAR(32) NOT NULL,
  updated_at VARCHAR(32) NOT NULL,
  UNIQUE KEY uk_aa_georebal_leg (plan_id, seq),
  KEY idx_aa_georebal_leg_plan (plan_id, state)
);
