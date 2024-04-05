-- 分片目录、版本化路由、跨分片意图、步骤、桥接凭证。
-- 这些表是协议投影。账本科目行、围栏令牌和补账分录不在这里改。
CREATE TABLE IF NOT EXISTS led_shard_shards (
-- 作者：Kinsley
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(16) NOT NULL,
  status VARCHAR(16) NOT NULL,
  weight INT NOT NULL,
  pin_subject VARCHAR(64) NOT NULL DEFAULT '',
  region VARCHAR(16) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_ms BIGINT NOT NULL,
  updated_ms BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS led_shard_routes (
  version BIGINT NOT NULL PRIMARY KEY,
  status VARCHAR(16) NOT NULL,
  modulus INT NOT NULL,
  user_shards TEXT NOT NULL,
  pins TEXT NOT NULL,
  affinity TEXT NOT NULL,
  fingerprint CHAR(64) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_ms BIGINT NOT NULL,
  published_ms BIGINT NOT NULL DEFAULT 0,
  activated_ms BIGINT NOT NULL DEFAULT 0,
  retired_ms BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS led_shard_intents (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  idem VARCHAR(128) NOT NULL,
  biz_hash CHAR(64) NOT NULL,
  route_version BIGINT NOT NULL,
  route_fp CHAR(64) NOT NULL,
  src_user VARCHAR(64) NOT NULL,
  src_book VARCHAR(64) NOT NULL,
  src_subject VARCHAR(128) NOT NULL,
  src_shard VARCHAR(64) NOT NULL,
  dst_user VARCHAR(64) NOT NULL,
  dst_book VARCHAR(64) NOT NULL,
  dst_subject VARCHAR(128) NOT NULL,
  dst_shard VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  state VARCHAR(32) NOT NULL,
  halted TINYINT NOT NULL DEFAULT 0,
  manual TINYINT NOT NULL DEFAULT 0,
  deadline_ms BIGINT NOT NULL DEFAULT 0,
  freeze_posted TINYINT NOT NULL DEFAULT 0,
  voucher_id VARCHAR(64) NOT NULL DEFAULT '',
  credit_posted TINYINT NOT NULL DEFAULT 0,
  credit_count INT NOT NULL DEFAULT 0,
  reverse_count INT NOT NULL DEFAULT 0,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  desk_id VARCHAR(64) NOT NULL DEFAULT '',
  created_ms BIGINT NOT NULL,
  updated_ms BIGINT NOT NULL,
  UNIQUE KEY uk_led_shard_idem (idem),
  KEY idx_led_shard_intent_state (state, updated_ms)
);

CREATE TABLE IF NOT EXISTS led_shard_steps (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  intent_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  state VARCHAR(16) NOT NULL,
  shard_id VARCHAR(64) NOT NULL DEFAULT '',
  ledger_from VARCHAR(32) NOT NULL DEFAULT '',
  ledger_to VARCHAR(32) NOT NULL DEFAULT '',
  detail VARCHAR(255) NOT NULL DEFAULT '',
  at_ms BIGINT NOT NULL,
  UNIQUE KEY uk_led_shard_step (intent_id, seq)
);

CREATE TABLE IF NOT EXISTS led_shard_vouchers (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  intent_id VARCHAR(64) NOT NULL,
  idem VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  src_shard VARCHAR(64) NOT NULL,
  dst_shard VARCHAR(64) NOT NULL,
  bridge_shard VARCHAR(64) NOT NULL,
  state VARCHAR(16) NOT NULL,
  route_version BIGINT NOT NULL,
  credit_count INT NOT NULL DEFAULT 0,
  created_ms BIGINT NOT NULL,
  consumed_ms BIGINT NOT NULL DEFAULT 0,
  voided_ms BIGINT NOT NULL DEFAULT 0,
  UNIQUE KEY uk_led_shard_voucher_intent (intent_id)
);

CREATE TABLE IF NOT EXISTS led_shard_balances (
  shard_id VARCHAR(64) NOT NULL,
  subject VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  available DECIMAL(36,18) NOT NULL,
  frozen DECIMAL(36,18) NOT NULL,
  PRIMARY KEY (shard_id, subject, asset)
);

CREATE TABLE IF NOT EXISTS led_shard_postings (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  intent_id VARCHAR(64) NOT NULL,
  voucher_id VARCHAR(64) NOT NULL DEFAULT '',
  seq INT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  shard_id VARCHAR(64) NOT NULL,
  subject VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  debit_avail DECIMAL(36,18) NOT NULL,
  credit_avail DECIMAL(36,18) NOT NULL,
  debit_frozen DECIMAL(36,18) NOT NULL,
  credit_frozen DECIMAL(36,18) NOT NULL,
  memo VARCHAR(64) NOT NULL DEFAULT '',
  at_ms BIGINT NOT NULL,
  KEY idx_led_shard_post_intent (intent_id, seq)
);

CREATE TABLE IF NOT EXISTS led_shard_idem (
  idem VARCHAR(128) NOT NULL PRIMARY KEY,
  biz_hash CHAR(64) NOT NULL,
  intent_id VARCHAR(64) NOT NULL
);

CREATE TABLE IF NOT EXISTS led_shard_alerts (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  intent_id VARCHAR(64) NOT NULL DEFAULT '',
  detail VARCHAR(255) NOT NULL DEFAULT '',
  acked TINYINT NOT NULL DEFAULT 0,
  at_ms BIGINT NOT NULL,
  KEY idx_led_shard_alert_kind (kind, at_ms)
);

CREATE TABLE IF NOT EXISTS led_shard_desk (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  intent_id VARCHAR(64) NOT NULL,
  state VARCHAR(16) NOT NULL,
  opened_by VARCHAR(64) NOT NULL,
  approved_by VARCHAR(64) NOT NULL DEFAULT '',
  executed_by VARCHAR(64) NOT NULL DEFAULT '',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  created_ms BIGINT NOT NULL,
  updated_ms BIGINT NOT NULL,
  KEY idx_led_shard_desk_intent (intent_id)
);

CREATE TABLE IF NOT EXISTS led_shard_hints (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  intent_id VARCHAR(64) NOT NULL,
  voucher_id VARCHAR(64) NOT NULL DEFAULT '',
  src_shard VARCHAR(64) NOT NULL,
  dst_shard VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  at_ms BIGINT NOT NULL
);
