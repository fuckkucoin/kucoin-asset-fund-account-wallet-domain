-- 账本科目、充提单据、地址册、出箱、托管钱包与链上历史。 Open 必须落这些表。
CREATE TABLE IF NOT EXISTS ledger_accounts (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
-- 作者：TianpengLan
  user_id VARCHAR(64) NOT NULL,
  account_type VARCHAR(32) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  available DECIMAL(36,18) NOT NULL DEFAULT 0,
  frozen DECIMAL(36,18) NOT NULL DEFAULT 0,
  UNIQUE KEY uk_ledger_acct (user_id, account_type, asset),
  CONSTRAINT chk_ledger_acct_nonneg CHECK (available >= 0 AND frozen >= 0)
);
CREATE TABLE IF NOT EXISTS transfers (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  idempotency_key VARCHAR(128) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  direction VARCHAR(32) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  to_address_enc BLOB,
  to_address_hint VARCHAR(32) NOT NULL DEFAULT '',
  tx_hash VARCHAR(128) NOT NULL DEFAULT '',
  state VARCHAR(32) NOT NULL,
  risk_flags JSON,
  fail_reason VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  observed_amount DECIMAL(36,18) NULL,
  observed_asset VARCHAR(16) NOT NULL DEFAULT '',
  credited_height BIGINT NOT NULL DEFAULT 0,
  credited_block VARCHAR(128) NOT NULL DEFAULT '',
  kyt_decision VARCHAR(32) NOT NULL DEFAULT '',
  UNIQUE KEY uk_transfers_idem (idempotency_key),
  KEY idx_transfers_user (user_id, created_at),
  KEY idx_transfers_state (state, created_at),
  KEY idx_transfers_chain_hash (chain, tx_hash)
);
CREATE TABLE IF NOT EXISTS ledger_journal (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  transfer_id VARCHAR(64) NOT NULL,
  entry_seq INT NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  account_type VARCHAR(32) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  debit DECIMAL(36,18) NOT NULL DEFAULT 0,
  credit DECIMAL(36,18) NOT NULL DEFAULT 0,
  memo VARCHAR(255) NOT NULL DEFAULT '',
  UNIQUE KEY uk_journal_seq (transfer_id, entry_seq),
  KEY idx_journal_user (user_id)
);
CREATE TABLE IF NOT EXISTS audit_log (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  entity_type VARCHAR(32) NOT NULL,
  entity_id VARCHAR(64) NOT NULL,
  action VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  detail BLOB,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_audit_entity (entity_type, entity_id)
);
CREATE TABLE IF NOT EXISTS address_allowlist (
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  label VARCHAR(64) NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, chain, address)
);
CREATE TABLE IF NOT EXISTS outbox_events (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  aggregate_type VARCHAR(32) NOT NULL,
  aggregate_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  action VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  trace_id VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL DEFAULT '',
  payload BLOB,
  published_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_outbox_pub (published_at, created_at)
);
CREATE TABLE IF NOT EXISTS user_controls (
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  frozen TINYINT NOT NULL DEFAULT 0,
  kyc_level VARCHAR(16) NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS custody_wallets (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  label VARCHAR(64) NOT NULL DEFAULT '',
  source VARCHAR(16) NOT NULL DEFAULT '',
  mnemonic_enc BLOB,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_custody_user (user_id)
);
CREATE TABLE IF NOT EXISTS custody_addresses (
  wallet_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  PRIMARY KEY (wallet_id, chain),
  UNIQUE KEY uk_custody_addr (address)
);
CREATE TABLE IF NOT EXISTS chain_tx_history (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL DEFAULT '',
  tx_hash VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  direction VARCHAR(8) NOT NULL,
  status VARCHAR(16) NOT NULL,
  confirmed_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_chain_tx (chain, tx_hash)
);
CREATE TABLE IF NOT EXISTS submit_nonces (
  nonce VARCHAR(128) NOT NULL PRIMARY KEY,
  tx_hash VARCHAR(128) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);
