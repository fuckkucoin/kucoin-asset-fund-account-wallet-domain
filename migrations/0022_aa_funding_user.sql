-- 资金账户用户侧：交易密码、禁出金、Memo 工单、入账科目、白名单模式、子户划转权限。
-- 内存装配走 persist.KV； SQLKV 与本表并存，本表为审计权威。

-- 作者：Kinsley
CREATE TABLE IF NOT EXISTS aa_trade_pw (
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  salt CHAR(32) NOT NULL,
  pin_hash CHAR(64) NOT NULL,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_wd_forbid (
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  forbidden TINYINT(1) NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_memo_tickets (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(32) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  memo VARCHAR(128) NOT NULL,
  address VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_memo_user (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS aa_deposit_to (
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(32) NOT NULL,
  book VARCHAR(16) NOT NULL,
  PRIMARY KEY (user_id, chain)
);

CREATE TABLE IF NOT EXISTS aa_whitelist_mode (
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  enabled TINYINT(1) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS aa_sub_xfer_perm (
  parent_id VARCHAR(64) NOT NULL,
  sub_user_id VARCHAR(64) NOT NULL,
  transferable TINYINT(1) NOT NULL DEFAULT 1,
  PRIMARY KEY (parent_id, sub_user_id)
);

CREATE TABLE IF NOT EXISTS aa_convert_orders (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  from_asset VARCHAR(16) NOT NULL,
  to_asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  rate VARCHAR(32) NOT NULL,
  book VARCHAR(16) NOT NULL,
  status VARCHAR(16) NOT NULL,
  debit_id VARCHAR(64) NOT NULL DEFAULT '',
  credit_id VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_convert_user (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS aa_export_jobs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  kind VARCHAR(16) NOT NULL,
  format VARCHAR(8) NOT NULL,
  status VARCHAR(16) NOT NULL,
  body MEDIUMTEXT,
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_export_user (user_id, created_at)
);
