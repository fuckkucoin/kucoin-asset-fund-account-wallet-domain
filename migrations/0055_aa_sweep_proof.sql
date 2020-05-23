-- 热温冷归集计划、腿、证明链、补偿和冷引用。哈希链按 plan_id+seq 主键追加；冷引用单独成表。这些表不是出金审计，不得拿来改 aa_audit_* 或账本围栏。
CREATE TABLE IF NOT EXISTS aa_sweep_proof_plans (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
-- 作者：Axel
  idem VARCHAR(128) NOT NULL,
  fingerprint CHAR(64) NOT NULL,
  chain VARCHAR(32) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  state VARCHAR(32) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  code VARCHAR(64) NOT NULL DEFAULT '',
  held VARCHAR(64) NOT NULL DEFAULT '0',
  head CHAR(64) NOT NULL DEFAULT '',
  cold_deadline DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_sweep_proof_idem (idem),
  KEY idx_aa_sweep_proof_asset (asset, created_at)
);

CREATE TABLE IF NOT EXISTS aa_sweep_proof_legs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  src_kind VARCHAR(32) NOT NULL DEFAULT '',
  from_tier VARCHAR(16) NOT NULL,
  to_tier VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  cold TINYINT NOT NULL DEFAULT 0,
  state VARCHAR(32) NOT NULL,
  parent_id VARCHAR(64) NOT NULL DEFAULT '',
  sign_state VARCHAR(32) NOT NULL DEFAULT '',
  bound_by VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_sweep_proof_leg (plan_id, seq),
  KEY idx_aa_sweep_proof_leg_plan (plan_id, state)
);

CREATE TABLE IF NOT EXISTS aa_sweep_proof_links (
  plan_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  prev_hash CHAR(64) NOT NULL,
  hash CHAR(64) NOT NULL,
  leg_id VARCHAR(64) NOT NULL DEFAULT '',
  leg_seq INT NOT NULL DEFAULT 0,
  kind VARCHAR(32) NOT NULL DEFAULT '',
  from_tier VARCHAR(16) NOT NULL DEFAULT '',
  to_tier VARCHAR(16) NOT NULL DEFAULT '',
  amount VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(32) NOT NULL,
  ceremony_id VARCHAR(64) NOT NULL DEFAULT '',
  airgap_id VARCHAR(64) NOT NULL DEFAULT '',
  sign_id VARCHAR(64) NOT NULL DEFAULT '',
  ceremony_phase VARCHAR(32) NOT NULL DEFAULT '',
  airgap_phase VARCHAR(32) NOT NULL DEFAULT '',
  sign_state VARCHAR(32) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  PRIMARY KEY (plan_id, seq),
  UNIQUE KEY uk_aa_sweep_proof_hash (hash)
);

CREATE TABLE IF NOT EXISTS aa_sweep_proof_comps (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL,
  parent_leg VARCHAR(64) NOT NULL,
  comp_leg VARCHAR(64) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  state VARCHAR(32) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_sweep_proof_comp_plan (plan_id, created_at)
);

CREATE TABLE IF NOT EXISTS aa_sweep_proof_refs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL,
  leg_id VARCHAR(64) NOT NULL,
  ceremony_id VARCHAR(64) NOT NULL,
  airgap_id VARCHAR(64) NOT NULL,
  sign_id VARCHAR(64) NOT NULL,
  ceremony_phase VARCHAR(32) NOT NULL DEFAULT '',
  airgap_phase VARCHAR(32) NOT NULL DEFAULT '',
  sign_state VARCHAR(32) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_sweep_proof_ref_leg (leg_id),
  KEY idx_aa_sweep_proof_ref_plan (plan_id, at)
);
