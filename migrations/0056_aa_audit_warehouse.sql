-- 功能：资金域证据仓。段、条目、检查点、摄取游标、重放作业、告警、受控导出。
-- 重点：条目只插入。段状态只允许 open→sealed→frozen。检查点按段序号追加。
-- 风险点：不要用可改历史的日志表代替本表；篡改后完整性巡检必须失败并挡住信任导出。
-- 作者：TianpengLan
CREATE TABLE IF NOT EXISTS aa_audit_segment (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  part_key VARCHAR(191) NOT NULL,
  domain VARCHAR(32) NOT NULL,
  tenant VARCHAR(64) NOT NULL DEFAULT '',
  day_key CHAR(8) NOT NULL,
  ordinal INT NOT NULL,
  state VARCHAR(16) NOT NULL,
  tier VARCHAR(8) NOT NULL DEFAULT 'hot',
  entry_count INT NOT NULL DEFAULT 0,
  head_hash CHAR(64) NOT NULL DEFAULT '',
  tail_hash CHAR(64) NOT NULL DEFAULT '',
  merkle_root CHAR(64) NOT NULL DEFAULT '',
  hold_reason VARCHAR(64) NOT NULL DEFAULT '',
  opened_at DATETIME(3) NOT NULL,
  sealed_at DATETIME(3) NULL,
  frozen_at DATETIME(3) NULL,
  UNIQUE KEY uk_aa_audit_seg_ord (part_key, ordinal)
);

CREATE TABLE IF NOT EXISTS aa_audit_entry (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  segment_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  idem_key VARCHAR(191) NOT NULL,
  source VARCHAR(32) NOT NULL,
  kind VARCHAR(48) NOT NULL,
  domain VARCHAR(32) NOT NULL,
  tenant VARCHAR(64) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  amount VARCHAR(64) NOT NULL DEFAULT '0',
  ref_id VARCHAR(128) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  prev_hash CHAR(64) NOT NULL DEFAULT '',
  self_hash CHAR(64) NOT NULL,
  occurred_at DATETIME(3) NOT NULL,
  ingested_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_audit_entry_seq (segment_id, seq),
  UNIQUE KEY uk_aa_audit_entry_idem (source, idem_key)
);

CREATE TABLE IF NOT EXISTS aa_audit_checkpoint (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  segment_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  merkle_root CHAR(64) NOT NULL,
  prev_root CHAR(64) NOT NULL DEFAULT '',
  entry_count INT NOT NULL,
  tip TINYINT NOT NULL DEFAULT 0,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_audit_ckpt (segment_id, seq, tip)
);

CREATE TABLE IF NOT EXISTS aa_audit_cursor (
  source VARCHAR(32) NOT NULL PRIMARY KEY,
  high_water VARCHAR(191) NOT NULL DEFAULT '',
  last_id VARCHAR(64) NOT NULL DEFAULT '',
  lag_ms BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_audit_replay_job (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  from_at DATETIME(3) NOT NULL,
  to_at DATETIME(3) NOT NULL,
  state VARCHAR(16) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  dual_id VARCHAR(64) NOT NULL DEFAULT '',
  report_json TEXT NOT NULL,
  error_text VARCHAR(255) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_audit_alert (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  segment_id VARCHAR(64) NOT NULL DEFAULT '',
  detail VARCHAR(255) NOT NULL DEFAULT '',
  open_flag TINYINT NOT NULL DEFAULT 1,
  at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_audit_export (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  segment_id VARCHAR(64) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  from_at DATETIME(3) NOT NULL,
  to_at DATETIME(3) NOT NULL,
  approval_id VARCHAR(64) NOT NULL DEFAULT '',
  requested_by VARCHAR(64) NOT NULL,
  state VARCHAR(16) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  released_at DATETIME(3) NULL
);
