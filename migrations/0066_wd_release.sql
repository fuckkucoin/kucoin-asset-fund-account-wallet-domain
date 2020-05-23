-- 功能：出金多级放行票据、分段、审批步、审计和策略版本。
-- 重点：这些表只记用户出金放行，不写 aa_approvals，也不写归集证明。
-- 风险点：把放行段标成已冻结却不经账本，会造成额度与余额脱节。
-- 作者：Bram
-- 日期：2021.11.23
CREATE TABLE IF NOT EXISTS wd_release_policy (
  version VARCHAR(32) NOT NULL PRIMARY KEY,
  active TINYINT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS wd_release_ticket (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  idem VARCHAR(128) NOT NULL,
  fingerprint CHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  chain VARCHAR(32) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  dest_class VARCHAR(32) NOT NULL,
  risk INT NOT NULL DEFAULT 0,
  policy_ver VARCHAR(32) NOT NULL,
  state VARCHAR(32) NOT NULL,
  depth INT NOT NULL,
  base_depth INT NOT NULL DEFAULT 0,
  approvals INT NOT NULL DEFAULT 0,
  skip TINYINT NOT NULL DEFAULT 0,
  timeout_action VARCHAR(16) NOT NULL,
  sla_sec BIGINT NOT NULL DEFAULT 0,
  deadline DATETIME(3) NULL,
  applicant VARCHAR(64) NOT NULL,
  code VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  authorized DECIMAL(36,18) NOT NULL DEFAULT 0,
  frozen DECIMAL(36,18) NOT NULL DEFAULT 0,
  residual DECIMAL(36,18) NOT NULL DEFAULT 0,
  escalated INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_wd_release_idem (idem),
  KEY idx_wd_release_state (state, deadline),
  KEY idx_wd_release_user (user_id, asset, created_at)
);

CREATE TABLE IF NOT EXISTS wd_release_segment (
  ticket_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  bps INT NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  cap DECIMAL(36,18) NULL,
  need_level INT NOT NULL,
  state VARCHAR(32) NOT NULL,
  authorized_by VARCHAR(64) NOT NULL DEFAULT '',
  at DATETIME(3) NULL,
  PRIMARY KEY (ticket_id, seq)
);

CREATE TABLE IF NOT EXISTS wd_release_step (
  ticket_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  level INT NOT NULL DEFAULT 0,
  actor VARCHAR(64) NOT NULL,
  role VARCHAR(32) NOT NULL DEFAULT '',
  code VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  PRIMARY KEY (ticket_id, seq)
);

CREATE TABLE IF NOT EXISTS wd_release_audit (
  ticket_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  event VARCHAR(32) NOT NULL,
  state VARCHAR(32) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  code VARCHAR(64) NOT NULL DEFAULT '',
  prev_hash CHAR(64) NOT NULL,
  hash CHAR(64) NOT NULL,
  at DATETIME(3) NOT NULL,
  PRIMARY KEY (ticket_id, seq),
  UNIQUE KEY uk_wd_release_audit_hash (hash)
);

CREATE TABLE IF NOT EXISTS wd_release_freeze (
  idem VARCHAR(128) NOT NULL PRIMARY KEY,
  ticket_id VARCHAR(64) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  phase VARCHAR(16) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_wd_release_freeze_ticket (ticket_id, phase)
);
