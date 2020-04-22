-- 一等表：异常工单、子户授权、归集计划、双控投票、策略元数据、KYT 证据。
--  Open 以这些表为权威；aa_kv 仅作回读/回填。

-- 作者：Axel
CREATE TABLE IF NOT EXISTS aa_dep_tickets (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  tx_hash VARCHAR(128) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL,
  state VARCHAR(16) NOT NULL,
  opened_by VARCHAR(64) NOT NULL,
  assignee VARCHAR(64) NOT NULL DEFAULT '',
  closed_by VARCHAR(64) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_dep_tickets_user (user_id, state, created_at)
);

CREATE TABLE IF NOT EXISTS aa_wd_tickets (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL,
  state VARCHAR(16) NOT NULL,
  opened_by VARCHAR(64) NOT NULL,
  assignee VARCHAR(64) NOT NULL DEFAULT '',
  closed_by VARCHAR(64) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_wd_tickets_user (user_id, state, created_at)
);

CREATE TABLE IF NOT EXISTS aa_recon_tickets (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  book VARCHAR(32) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL,
  state VARCHAR(16) NOT NULL,
  opened_by VARCHAR(64) NOT NULL,
  acked_by VARCHAR(64) NOT NULL DEFAULT '',
  closed_by VARCHAR(64) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_recon_tickets_user (user_id, state, created_at)
);

CREATE TABLE IF NOT EXISTS aa_ticket_events (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  scope VARCHAR(16) NOT NULL,
  ticket_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  event VARCHAR(32) NOT NULL,
  from_state VARCHAR(16) NOT NULL DEFAULT '',
  to_state VARCHAR(16) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_ticket_events (scope, ticket_id, at)
);

CREATE TABLE IF NOT EXISTS aa_subacct_parents (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  label VARCHAR(64) NOT NULL,
  status VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  opened_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_subacct_parent_user (user_id)
);

CREATE TABLE IF NOT EXISTS aa_subacct_children (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  parent_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  label VARCHAR(64) NOT NULL,
  status VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  privileges VARCHAR(128) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  opened_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_subacct_child_user (user_id),
  KEY idx_aa_subacct_child_parent (parent_id)
);

CREATE TABLE IF NOT EXISTS aa_subacct_grants (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  child_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  reviewer VARCHAR(64) NOT NULL DEFAULT '',
  add_privs VARCHAR(128) NOT NULL DEFAULT '',
  remove_privs VARCHAR(128) NOT NULL DEFAULT '',
  assets VARCHAR(255) NOT NULL DEFAULT '',
  daily_cap DECIMAL(36,18) NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_subacct_grants_child (child_id, created_at)
);

CREATE TABLE IF NOT EXISTS aa_subacct_hops (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  idem VARCHAR(128) NOT NULL,
  from_owner VARCHAR(64) NOT NULL,
  to_owner VARCHAR(64) NOT NULL,
  from_user VARCHAR(64) NOT NULL DEFAULT '',
  to_user VARCHAR(64) NOT NULL DEFAULT '',
  from_book VARCHAR(32) NOT NULL DEFAULT '',
  to_book VARCHAR(32) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  kind VARCHAR(32) NOT NULL DEFAULT '',
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_subacct_hop_idem (idem)
);

CREATE TABLE IF NOT EXISTS aa_sweep_schedules (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  region VARCHAR(16) NOT NULL DEFAULT 'sg',
  weekdays VARCHAR(32) NOT NULL DEFAULT '',
  start_utc SMALLINT NOT NULL DEFAULT 0,
  end_utc SMALLINT NOT NULL DEFAULT 0,
  max_plans INT NOT NULL DEFAULT 8,
  halt TINYINT NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_sweep_sched_region (region)
);

CREATE TABLE IF NOT EXISTS aa_sweep_plans (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  schedule_id VARCHAR(64) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL DEFAULT 'planned',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_sweep_plans_asset (asset, created_at)
);

CREATE TABLE IF NOT EXISTS aa_dualctl_requests (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  subject VARCHAR(128) NOT NULL DEFAULT '',
  opened_by VARCHAR(64) NOT NULL,
  need INT NOT NULL,
  state VARCHAR(16) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  expires_at DATETIME(3) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_dualctl_state (state, created_at)
);

CREATE TABLE IF NOT EXISTS aa_dualctl_votes (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  request_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  allow TINYINT NOT NULL,
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_dualctl_vote (request_id, actor),
  KEY idx_aa_dualctl_votes_req (request_id)
);

CREATE TABLE IF NOT EXISTS aa_policy_meta (
  pack VARCHAR(32) NOT NULL,
  version INT NOT NULL,
  kyc VARCHAR(16) NOT NULL DEFAULT '',
  dest_reuse_max INT NOT NULL DEFAULT 0,
  hourly_velocity INT NOT NULL DEFAULT 0,
  daily_velocity INT NOT NULL DEFAULT 0,
  checksum VARCHAR(64) NOT NULL DEFAULT '',
  published_at DATETIME(3) NOT NULL,
  PRIMARY KEY (pack, version)
);

CREATE TABLE IF NOT EXISTS aa_kyt_evidence (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  hit_id VARCHAR(64) NOT NULL DEFAULT '',
  movement_id VARCHAR(128) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL,
  body TEXT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_kyt_evidence_move (movement_id, created_at),
  KEY idx_aa_kyt_evidence_hit (hit_id)
);
