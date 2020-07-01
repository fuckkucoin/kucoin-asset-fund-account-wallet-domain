-- 功能：偿付证明负债叶、准备金快照、证明单、漂移工单与调度。
-- 重点：证明状态 building/sealed/published/revoked/abandoned；漂移 open/acked/closed。
-- 风险点：building 与封存摘要为空的行不得更新为 published；观察失败不得写成已覆盖。
-- 作者：TianpengLan
CREATE TABLE IF NOT EXISTS aa_solvency_schedules (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  interval_sec BIGINT NOT NULL,
  window_sec BIGINT NOT NULL,
  assets VARCHAR(255) NOT NULL DEFAULT '',
  enabled TINYINT NOT NULL DEFAULT 1,
  auto_publish TINYINT NOT NULL DEFAULT 0,
  next_due DATETIME(3) NOT NULL,
  last_run_id VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_solvency_attestations (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  schedule_id VARCHAR(64) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  class_name VARCHAR(32) NOT NULL,
  root_hash CHAR(64) NOT NULL DEFAULT '',
  leaf_count INT NOT NULL DEFAULT 0,
  signature VARCHAR(128) NOT NULL DEFAULT '',
  signer VARCHAR(64) NOT NULL DEFAULT '',
  window_start DATETIME(3) NOT NULL,
  window_end DATETIME(3) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  book_digest CHAR(64) NOT NULL DEFAULT '',
  seal_digest CHAR(64) NOT NULL DEFAULT '',
  coverage VARCHAR(16) NOT NULL DEFAULT '',
  scope_assets VARCHAR(255) NOT NULL DEFAULT '',
  disclaimer VARCHAR(255) NOT NULL DEFAULT '',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  audit_ref VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  sealed_at DATETIME(3) NULL,
  published_at DATETIME(3) NULL,
  revoked_at DATETIME(3) NULL,
  KEY idx_aa_solv_att_sched (schedule_id, window_start),
  KEY idx_aa_solv_att_status (status, updated_at)
);

CREATE TABLE IF NOT EXISTS aa_solvency_leaves (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  attestation_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  book VARCHAR(32) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  available VARCHAR(64) NOT NULL,
  frozen VARCHAR(64) NOT NULL,
  total VARCHAR(64) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  leaf_hash CHAR(64) NOT NULL,
  collected_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_solv_leaf (attestation_id, user_id, book, asset),
  KEY idx_aa_solv_leaf_att (attestation_id)
);

CREATE TABLE IF NOT EXISTS aa_solvency_reserve_snaps (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  attestation_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  hot VARCHAR(64) NOT NULL,
  warm VARCHAR(64) NOT NULL,
  cold VARCHAR(64) NOT NULL,
  observed VARCHAR(64) NOT NULL,
  liability VARCHAR(64) NOT NULL,
  surplus VARCHAR(64) NOT NULL,
  deficit VARCHAR(64) NOT NULL,
  source_name VARCHAR(32) NOT NULL,
  taken_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_solv_rsv (attestation_id, asset),
  KEY idx_aa_solv_rsv_att (attestation_id)
);

CREATE TABLE IF NOT EXISTS aa_solvency_drifts (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  attestation_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  status VARCHAR(16) NOT NULL,
  severity VARCHAR(16) NOT NULL,
  liability VARCHAR(64) NOT NULL,
  observed VARCHAR(64) NOT NULL,
  hot VARCHAR(64) NOT NULL DEFAULT '0',
  warm VARCHAR(64) NOT NULL DEFAULT '0',
  cold VARCHAR(64) NOT NULL DEFAULT '0',
  deficit VARCHAR(64) NOT NULL,
  ratio_bps BIGINT NOT NULL DEFAULT 0,
  explain_text VARCHAR(512) NOT NULL,
  opened_at DATETIME(3) NOT NULL,
  acked_at DATETIME(3) NULL,
  closed_at DATETIME(3) NULL,
  acked_by VARCHAR(64) NOT NULL DEFAULT '',
  closed_by VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  signaled TINYINT NOT NULL DEFAULT 0,
  UNIQUE KEY uk_aa_solv_drift_asset (attestation_id, asset),
  KEY idx_aa_solv_drift_status (status, opened_at)
);
