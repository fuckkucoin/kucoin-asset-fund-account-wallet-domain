-- 逐仓交易对、子账户 API Key 密文引用、UTA 资金侧科目。以 SQL 为权威。

CREATE TABLE IF NOT EXISTS aa_isolated_pairs (
-- 作者：Axel
  user_id VARCHAR(64) NOT NULL,
  symbol VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  debt_ratio VARCHAR(32) NOT NULL DEFAULT '0',
  base_currency VARCHAR(16) NOT NULL,
  base_total DECIMAL(36,18) NOT NULL DEFAULT 0,
  base_available DECIMAL(36,18) NOT NULL DEFAULT 0,
  base_hold DECIMAL(36,18) NOT NULL DEFAULT 0,
  base_liability DECIMAL(36,18) NOT NULL DEFAULT 0,
  quote_currency VARCHAR(16) NOT NULL,
  quote_total DECIMAL(36,18) NOT NULL DEFAULT 0,
  quote_available DECIMAL(36,18) NOT NULL DEFAULT 0,
  quote_hold DECIMAL(36,18) NOT NULL DEFAULT 0,
  quote_liability DECIMAL(36,18) NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (user_id, symbol)
);

CREATE TABLE IF NOT EXISTS aa_sub_api_keys (
  api_key VARCHAR(64) NOT NULL PRIMARY KEY,
  owner VARCHAR(64) NOT NULL,
  sub_name VARCHAR(64) NOT NULL,
  remark VARCHAR(128) NOT NULL DEFAULT '',
  api_version INT NOT NULL DEFAULT 3,
  permission VARCHAR(64) NOT NULL DEFAULT 'General',
  ip_whitelist VARCHAR(255) NOT NULL DEFAULT '',
  pass_hash CHAR(64) NOT NULL,
  secret_enc BLOB NOT NULL,
  created_at BIGINT NOT NULL,
  KEY idx_aa_sub_api_owner (owner, sub_name)
);

CREATE TABLE IF NOT EXISTS aa_uta_mode (
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  account_mode VARCHAR(16) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_uta_leverage (
  user_id VARCHAR(64) NOT NULL,
  symbol VARCHAR(32) NOT NULL DEFAULT '',
  currency VARCHAR(16) NOT NULL DEFAULT '',
  margin_mode VARCHAR(16) NOT NULL DEFAULT 'CROSS',
  trade_type VARCHAR(16) NOT NULL DEFAULT '',
  leverage VARCHAR(16) NOT NULL,
  PRIMARY KEY (user_id, symbol, currency, margin_mode, trade_type)
);

CREATE TABLE IF NOT EXISTS aa_uta_interest (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  currency VARCHAR(16) NOT NULL,
  amount VARCHAR(32) NOT NULL,
  created_at BIGINT NOT NULL,
  KEY idx_aa_uta_interest (user_id, currency, created_at)
);
