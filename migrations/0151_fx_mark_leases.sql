-- 法币牌价租约 / 漂移证明 / 审计 / 世代。
-- 作者：King  日期：2021.11.24

CREATE TABLE IF NOT EXISTS fx_mark_leases (
  id VARCHAR(64) PRIMARY KEY,
  from_asset VARCHAR(16) NOT NULL,
  to_asset VARCHAR(16) NOT NULL,
  pair_key VARCHAR(40) NOT NULL,
  rate VARCHAR(64) NOT NULL,
  source VARCHAR(16) NOT NULL,
  source_print VARCHAR(80) NOT NULL,
  generation BIGINT NOT NULL,
  state VARCHAR(16) NOT NULL,
  ttl_ms BIGINT NOT NULL,
  issued_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  bound_at DATETIME NULL,
  consumed_at DATETIME NULL,
  expired_at DATETIME NULL,
  revoked_at DATETIME NULL,
  proof_id VARCHAR(64) NULL,
  idem VARCHAR(80) NULL,
  revoke_reason VARCHAR(255) NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  INDEX idx_fx_mark_leases_pair (pair_key, issued_at),
  INDEX idx_fx_mark_leases_state (state, expires_at),
  INDEX idx_fx_mark_leases_idem (idem)
);

CREATE TABLE IF NOT EXISTS fx_mark_drift_proofs (
  id VARCHAR(64) PRIMARY KEY,
  lease_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_fx_mark_proofs_lease (lease_id, created_at)
);

CREATE TABLE IF NOT EXISTS fx_mark_audit (
  id VARCHAR(64) PRIMARY KEY,
  lease_id VARCHAR(64) NOT NULL,
  event VARCHAR(16) NOT NULL,
  from_state VARCHAR(16) NOT NULL,
  to_state VARCHAR(16) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  body JSON NOT NULL,
  at DATETIME NOT NULL,
  INDEX idx_fx_mark_audit_lease (lease_id, at)
);

CREATE TABLE IF NOT EXISTS fx_mark_generations (
  asset VARCHAR(16) PRIMARY KEY,
  seq BIGINT NOT NULL,
  last_usd VARCHAR(64) NOT NULL,
  updated_at DATETIME NOT NULL
);
