-- 出金白名单、充值地址库存、上架合约、代币簿。 Open 以这些表为权威。

CREATE TABLE IF NOT EXISTS aa_dest_allow (
-- 作者：TianpengLan
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  label VARCHAR(64) NOT NULL DEFAULT '',
  added_at DATETIME(3) NOT NULL,
  PRIMARY KEY (user_id, chain, address),
  KEY idx_aa_dest_allow_user (user_id, chain)
);

CREATE TABLE IF NOT EXISTS aa_dest_allow_gen (
  scope VARCHAR(64) NOT NULL PRIMARY KEY,
  gen BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_addr_slots (
  address VARCHAR(128) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  family VARCHAR(16) NOT NULL,
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  reserved_at DATETIME(3) NULL,
  bound_at DATETIME(3) NULL,
  cool_until DATETIME(3) NULL,
  KEY idx_aa_addr_slots_chain (chain, status)
);

CREATE TABLE IF NOT EXISTS aa_listing_assets (
  symbol VARCHAR(16) NOT NULL PRIMARY KEY,
  decimals INT NOT NULL,
  min_withdraw VARCHAR(64) NOT NULL DEFAULT '',
  min_deposit VARCHAR(64) NOT NULL DEFAULT '',
  deposit_enabled TINYINT NOT NULL DEFAULT 1,
  withdraw_enabled TINYINT NOT NULL DEFAULT 1,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_listing_binds (
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  contract VARCHAR(128) NOT NULL,
  decimals INT NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (asset, chain)
);

CREATE TABLE IF NOT EXISTS aa_token_book (
  chain VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  contract VARCHAR(128) NOT NULL,
  decimals INT NOT NULL,
  PRIMARY KEY (chain, asset)
);
