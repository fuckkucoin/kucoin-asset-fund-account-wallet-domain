// persist/schema_pg.go

// Author: Axel
// Date: 2020.09.08

package persist

// SchemaPostgres 与 SchemaSQL 同表。类型按 Postgres；金额仍 NUMERIC(36,18)。
// 含 address_allowlist.memo / aa_earn_products.deleted_at（0032）与 aa_earn_product_audit（0033）、aa_earn_product_audit_cold（0035）。
// (chain, tx_hash) 主键，禁止两笔入金认领同一哈希。

const SchemaPostgres = `
CREATE TABLE IF NOT EXISTS aa_fences (
  row_id VARCHAR(191) NOT NULL PRIMARY KEY,
  epoch BIGINT NOT NULL DEFAULT 0,
  token VARCHAR(64) NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_cases (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  movement_id VARCHAR(128) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  reason VARCHAR(255) NOT NULL,
  state VARCHAR(32) NOT NULL,
  assignee VARCHAR(64) NOT NULL DEFAULT '',
  decided_by VARCHAR(64) NOT NULL DEFAULT '',
  policy_ver INT NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aa_cases_move ON aa_cases (movement_id);
CREATE INDEX IF NOT EXISTS idx_aa_cases_state ON aa_cases (state, updated_at);
CREATE TABLE IF NOT EXISTS aa_case_audit (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  case_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  event VARCHAR(32) NOT NULL,
  from_state VARCHAR(32) NOT NULL DEFAULT '',
  to_state VARCHAR(32) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aa_case_audit ON aa_case_audit (case_id, at);
CREATE TABLE IF NOT EXISTS aa_usage_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(16) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL DEFAULT '',
  amount NUMERIC(36,18) NOT NULL,
  idem VARCHAR(128) NOT NULL DEFAULT '',
  at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_aa_usage_idem ON aa_usage_hits (idem);
CREATE INDEX IF NOT EXISTS idx_aa_usage_user ON aa_usage_hits (kind, user_id, asset, at);
CREATE TABLE IF NOT EXISTS aa_addr_binds (
  bind_key VARCHAR(191) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  family VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  allocated SMALLINT NOT NULL DEFAULT 1,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  bound_at TIMESTAMPTZ NOT NULL,
  reuse_after TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_aa_addr ON aa_addr_binds (address);
CREATE TABLE IF NOT EXISTS aa_approvals (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(16) NOT NULL,
  movement_id VARCHAR(128) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL DEFAULT '',
  dest VARCHAR(128) NOT NULL DEFAULT '',
  amount NUMERIC(36,18) NOT NULL DEFAULT 0,
  requested_by VARCHAR(64) NOT NULL,
  approved_by VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_deposits (
  chain VARCHAR(16) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  output_index INT NOT NULL DEFAULT 0,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  amount NUMERIC(36,18) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  step VARCHAR(16) NOT NULL DEFAULT '',
  from_address VARCHAR(128) NOT NULL DEFAULT '',
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  run_id VARCHAR(64) NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (chain, tx_hash, output_index)
);
CREATE INDEX IF NOT EXISTS idx_aa_dep_xfer ON aa_deposits (transfer_id, step);
CREATE INDEX IF NOT EXISTS idx_aa_dep_user_asset_created ON aa_deposits (user_id, asset, created_at);
CREATE INDEX IF NOT EXISTS idx_aa_dep_step_updated ON aa_deposits (step, updated_at);
CREATE TABLE IF NOT EXISTS aa_withdraws (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount NUMERIC(36,18) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  idem VARCHAR(128) NOT NULL,
  state VARCHAR(32) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_aa_wd_idem ON aa_withdraws (idem);
CREATE TABLE IF NOT EXISTS aa_wd_outbound (
  transfer_id VARCHAR(64) NOT NULL PRIMARY KEY,
  run_id VARCHAR(64) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL DEFAULT '',
  amount NUMERIC(36,18) NOT NULL DEFAULT 0,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  idem VARCHAR(128) NOT NULL DEFAULT '',
  step VARCHAR(16) NOT NULL DEFAULT '',
  kyt VARCHAR(16) NOT NULL DEFAULT '',
  policy_ver INT NOT NULL DEFAULT 0,
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  debit_once SMALLINT NOT NULL DEFAULT 0,
  fail_sealed SMALLINT NOT NULL DEFAULT 0,
  compensate VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_aa_wd_outbound_idem ON aa_wd_outbound (idem);
CREATE INDEX IF NOT EXISTS idx_aa_wd_outbound_step ON aa_wd_outbound (step, updated_at);
CREATE INDEX IF NOT EXISTS idx_aa_wd_outbound_user ON aa_wd_outbound (user_id, asset, created_at);
CREATE TABLE IF NOT EXISTS aa_book_xfers (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  from_book VARCHAR(16) NOT NULL,
  to_book VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount NUMERIC(36,18) NOT NULL,
  idem VARCHAR(128) NOT NULL,
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_aa_xfer_idem ON aa_book_xfers (idem);
CREATE TABLE IF NOT EXISTS aa_audit_log (
  seq BIGSERIAL PRIMARY KEY,
  id VARCHAR(64) NOT NULL UNIQUE,
  kind VARCHAR(32) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  subject VARCHAR(128) NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_policy_docs (
  pack VARCHAR(32) NOT NULL,
  version INT NOT NULL,
  body TEXT NOT NULL,
  published_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (pack, version)
);
CREATE TABLE IF NOT EXISTS aa_kv (
  row_id VARCHAR(191) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  row_key VARCHAR(191) NOT NULL,
  body BYTEA,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  UNIQUE (kind, row_key)
);
CREATE TABLE IF NOT EXISTS aa_outbox (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  ev_key VARCHAR(128) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  payload TEXT NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  sent_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_aa_outbox_pending ON aa_outbox (sent_at, created_at);
CREATE TABLE IF NOT EXISTS aa_seq (
  name VARCHAR(64) NOT NULL PRIMARY KEY,
  n BIGINT NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_kyt_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  movement_id VARCHAR(128) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount NUMERIC(36,18) NOT NULL,
  decision VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  pack VARCHAR(32) NOT NULL DEFAULT '',
  pack_ver INT NOT NULL DEFAULT 0,
  mode VARCHAR(16) NOT NULL DEFAULT 'enforce',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aa_kyt_user ON aa_kyt_hits (user_id, created_at);
CREATE TABLE IF NOT EXISTS aa_custody_inv (
  asset VARCHAR(16) NOT NULL,
  side VARCHAR(8) NOT NULL,
  available NUMERIC(36,18) NOT NULL,
  frozen NUMERIC(36,18) NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  taken_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (asset, side)
);
CREATE TABLE IF NOT EXISTS aa_nonce_slots (
  slot_key VARCHAR(191) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  from_addr VARCHAR(128) NOT NULL,
  next_nonce BIGINT NOT NULL DEFAULT 0,
  last_hash VARCHAR(128) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_sign_jobs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  movement_id VARCHAR(128) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  amount NUMERIC(36,18) NOT NULL,
  canonical TEXT NOT NULL,
  state VARCHAR(16) NOT NULL,
  adapter VARCHAR(32) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_aa_sign_move ON aa_sign_jobs (movement_id) WHERE state IN ('queued','approved');
CREATE TABLE IF NOT EXISTS aa_policy_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  pack VARCHAR(32) NOT NULL,
  pack_ver INT NOT NULL,
  rule VARCHAR(64) NOT NULL,
  mode VARCHAR(16) NOT NULL,
  allow SMALLINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aa_policy_hits ON aa_policy_hits (pack, rule, created_at);
CREATE TABLE IF NOT EXISTS address_allowlist (
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  label VARCHAR(64) NOT NULL DEFAULT '',
  memo VARCHAR(128) NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, chain, address)
);
CREATE TABLE IF NOT EXISTS aa_earn_positions (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  principal VARCHAR(64) NOT NULL DEFAULT '0',
  accrued VARCHAR(64) NOT NULL DEFAULT '0',
  claimed VARCHAR(64) NOT NULL DEFAULT '0',
  penalty VARCHAR(64) NOT NULL DEFAULT '',
  opened_at TIMESTAMPTZ NOT NULL,
  unlock_at TIMESTAMPTZ NULL,
  last_accrue TIMESTAMPTZ NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'open',
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
  auto_subscribe SMALLINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aa_earn_pos_user ON aa_earn_positions (user_id, status);
CREATE INDEX IF NOT EXISTS idx_aa_earn_pos_idem ON aa_earn_positions (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_aa_earn_pos_product ON aa_earn_positions (product_id, status);
CREATE TABLE IF NOT EXISTS aa_earn_products (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  asset VARCHAR(16) NOT NULL,
  category VARCHAR(32) NOT NULL DEFAULT 'flexible',
  daily_bps INT NOT NULL DEFAULT 0,
  lock_days INT NOT NULL DEFAULT 0,
  min_subscribe VARCHAR(64) NOT NULL DEFAULT '',
  max_position VARCHAR(64) NOT NULL DEFAULT '',
  early_bps INT NOT NULL DEFAULT 0,
  active SMALLINT NOT NULL DEFAULT 1,
  auto_subscribe SMALLINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL,
  deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_aa_earn_prod_cat ON aa_earn_products (category, active);
CREATE INDEX IF NOT EXISTS idx_aa_earn_prod_deleted ON aa_earn_products (deleted_at);
CREATE TABLE IF NOT EXISTS aa_earn_product_audit (
  id BIGSERIAL PRIMARY KEY,
  product_id VARCHAR(64) NOT NULL,
  action VARCHAR(16) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aa_earn_prod_audit_prod ON aa_earn_product_audit (product_id, at);
CREATE INDEX IF NOT EXISTS idx_aa_earn_prod_audit_at ON aa_earn_product_audit (at);
CREATE TABLE IF NOT EXISTS aa_earn_product_audit_cold (
  id BIGSERIAL PRIMARY KEY,
  archive_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL DEFAULT '',
  path VARCHAR(512) NOT NULL,
  row_count INT NOT NULL DEFAULT 0,
  start_at TIMESTAMPTZ NOT NULL,
  end_at TIMESTAMPTZ NOT NULL,
  sha256 CHAR(64) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_aa_earn_audit_cold_id ON aa_earn_product_audit_cold (archive_id);
CREATE INDEX IF NOT EXISTS idx_aa_earn_audit_cold_prod ON aa_earn_product_audit_cold (product_id, at);
CREATE INDEX IF NOT EXISTS idx_aa_earn_audit_cold_at ON aa_earn_product_audit_cold (at);

CREATE TABLE IF NOT EXISTS aa_schema_version (
  version INT NOT NULL PRIMARY KEY,
  name VARCHAR(191) NOT NULL UNIQUE,
  checksum VARCHAR(64) NOT NULL DEFAULT '',
  applied_at TIMESTAMPTZ NOT NULL
);
`
