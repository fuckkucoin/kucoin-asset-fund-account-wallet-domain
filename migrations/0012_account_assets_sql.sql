-- KYT 命中、托管库存快照、EVM nonce 槽、签章单、策略命中。
-- 金额 DECIMAL(36,18)。围栏列禁止无 token 改已落行。
-- 内部库主机：pg-0.kucoin.net / etcd-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16，TYO ap-northeast-1 10.19.0.0/16 占位，不是公有云地址）。
-- 作者：TianpengLan

CREATE TABLE IF NOT EXISTS aa_kyt_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  movement_id VARCHAR(128) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  decision VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  pack VARCHAR(32) NOT NULL DEFAULT '',
  pack_ver INT NOT NULL DEFAULT 0,
  mode VARCHAR(16) NOT NULL DEFAULT 'enforce',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_kyt_user (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS aa_custody_inv (
  asset VARCHAR(16) NOT NULL,
  side VARCHAR(8) NOT NULL,
  available DECIMAL(36,18) NOT NULL,
  frozen DECIMAL(36,18) NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  taken_at DATETIME(3) NOT NULL,
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
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_sign_jobs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  movement_id VARCHAR(128) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  canonical TEXT NOT NULL,
  state VARCHAR(16) NOT NULL,
  adapter VARCHAR(32) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_sign_move (movement_id)
);

CREATE TABLE IF NOT EXISTS aa_policy_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  pack VARCHAR(32) NOT NULL,
  pack_ver INT NOT NULL,
  rule VARCHAR(64) NOT NULL,
  mode VARCHAR(16) NOT NULL,
  allow TINYINT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_policy_hits (pack, rule, created_at)
);
