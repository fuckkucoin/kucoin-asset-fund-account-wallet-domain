-- 功能：冷签仪式策略、会话、批次绑定、份额回执、审计、批次摘要和别名。
-- 重点：会话状态只由编排写入。有效绑定同一批次同时一行。别名不得改指向另一批次。
-- 风险点：摘要或份额被改掉后，签播闸必须拒绝；中止只释放绑定，不改出金广播行。
-- 作者：Kinsley
CREATE TABLE IF NOT EXISTS aa_cer_policy (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  purpose VARCHAR(32) NOT NULL,
  min_amount VARCHAR(64) NOT NULL,
  max_amount VARCHAR(64) NOT NULL DEFAULT '',
  dest_risk VARCHAR(16) NOT NULL,
  side VARCHAR(16) NOT NULL,
  force_cold TINYINT NOT NULL,
  threshold INT NOT NULL,
  shares INT NOT NULL,
  bind_mode VARCHAR(8) NOT NULL,
  priority INT NOT NULL,
  enabled TINYINT NOT NULL,
  approval_need TINYINT NOT NULL DEFAULT 0,
  max_age_sec INT NOT NULL DEFAULT 0,
  limit_ref VARCHAR(128) NOT NULL DEFAULT '',
  updated_by VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_cer_session (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  policy_id VARCHAR(64) NOT NULL,
  state VARCHAR(32) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  dest VARCHAR(191) NOT NULL,
  dest_risk VARCHAR(16) NOT NULL,
  airgap_id VARCHAR(64) NOT NULL DEFAULT '',
  keycer_ticket VARCHAR(64) NOT NULL DEFAULT '',
  custody_id VARCHAR(64) NOT NULL DEFAULT '',
  digest CHAR(64) NOT NULL DEFAULT '',
  starter VARCHAR(64) NOT NULL,
  last_actor VARCHAR(64) NOT NULL,
  deadline DATETIME(3) NOT NULL,
  abort_reason VARCHAR(255) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL,
  fence_token VARCHAR(64) NOT NULL,
  alerted_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_cer_ses_batch (batch_id, state),
  KEY idx_aa_cer_ses_state (state, deadline)
);

CREATE TABLE IF NOT EXISTS aa_cer_binding (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  batch_id VARCHAR(64) NOT NULL,
  session_id VARCHAR(64) NOT NULL,
  digest CHAR(64) NOT NULL,
  state VARCHAR(16) NOT NULL,
  bind_mode VARCHAR(8) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  bound_at DATETIME(3) NOT NULL,
  released_at DATETIME(3) NULL,
  KEY idx_aa_cer_bind_batch (batch_id, state),
  KEY idx_aa_cer_bind_ses (session_id, state)
);

CREATE TABLE IF NOT EXISTS aa_cer_receipt (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  session_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  share_ref CHAR(64) NOT NULL,
  slot_label VARCHAR(64) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_cer_rcpt_actor (session_id, actor)
);

CREATE TABLE IF NOT EXISTS aa_cer_audit (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  session_id VARCHAR(64) NOT NULL DEFAULT '',
  batch_id VARCHAR(64) NOT NULL DEFAULT '',
  kind VARCHAR(48) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  detail VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_cer_audit_ses (session_id, at),
  KEY idx_aa_cer_audit_batch (batch_id, at)
);

CREATE TABLE IF NOT EXISTS aa_cer_snap (
  batch_id VARCHAR(64) NOT NULL PRIMARY KEY,
  body TEXT NOT NULL,
  digest CHAR(64) NOT NULL,
  at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_cer_alias (
  ref_id VARCHAR(128) NOT NULL PRIMARY KEY,
  batch_id VARCHAR(64) NOT NULL,
  kind VARCHAR(16) NOT NULL
);
