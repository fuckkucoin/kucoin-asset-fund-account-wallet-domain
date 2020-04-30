-- 资金域幂等键与对账漂移闸持久化（出金/入账/hop 共用）。
CREATE TABLE IF NOT EXISTS aa_idempotency_key (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
-- 作者：TianpengLan
  scope VARCHAR(64) NOT NULL,
  idem_key VARCHAR(128) NOT NULL,
  fingerprint CHAR(64) NOT NULL,
  ref_id VARCHAR(64) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'inflight',
  payload_hash CHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_idem_scope_key (scope, idem_key),
  KEY idx_aa_idem_ref (ref_id),
  KEY idx_aa_idem_updated (updated_at)
);

CREATE TABLE IF NOT EXISTS aa_ledger_drift_block (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  reason VARCHAR(512) NOT NULL,
  report_json MEDIUMTEXT NULL,
  blocked_at DATETIME(3) NOT NULL,
  cleared_at DATETIME(3) NULL,
  clear_actor VARCHAR(64) NOT NULL DEFAULT '',
  clear_note VARCHAR(512) NOT NULL DEFAULT '',
  KEY idx_aa_drift_user_open (user_id, cleared_at),
  KEY idx_aa_drift_blocked (blocked_at)
);

CREATE TABLE IF NOT EXISTS aa_withdraw_freeze_event (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  transfer_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  from_state VARCHAR(32) NOT NULL,
  to_state VARCHAR(32) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  detail VARCHAR(512) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_wd_freeze_xfer (transfer_id, at),
  KEY idx_aa_wd_freeze_user (user_id, at)
);
