-- 出金手续费抬价与广播确认跨重启持久化；启动 Hydrate 回内存跟踪面。
CREATE TABLE IF NOT EXISTS aa_wd_fee_bumps (
  transfer_id VARCHAR(64) NOT NULL PRIMARY KEY,
-- 作者：Axel
  chain VARCHAR(32) NOT NULL DEFAULT '',
  old_hash VARCHAR(128) NOT NULL DEFAULT '',
  new_hash VARCHAR(128) NOT NULL DEFAULT '',
  old_fee VARCHAR(64) NOT NULL DEFAULT '0',
  new_fee VARCHAR(64) NOT NULL DEFAULT '0',
  replaced TINYINT(1) NOT NULL DEFAULT 0,
  bumped_at DATETIME(3) NOT NULL,
  KEY idx_aa_wd_fee_bumps_at (bumped_at)
);

CREATE TABLE IF NOT EXISTS aa_wd_broadcasts (
  transfer_id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(32) NOT NULL DEFAULT '',
  tx_hash VARCHAR(128) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  amount VARCHAR(64) NOT NULL DEFAULT '0',
  status VARCHAR(32) NOT NULL DEFAULT '',
  confirmations INT NOT NULL DEFAULT 0,
  need_conf INT NOT NULL DEFAULT 0,
  height BIGINT NOT NULL DEFAULT 0,
  block_hash VARCHAR(128) NOT NULL DEFAULT '',
  err VARCHAR(256) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_wd_broadcasts_st (status, updated_at),
  KEY idx_aa_wd_broadcasts_hash (tx_hash)
);
