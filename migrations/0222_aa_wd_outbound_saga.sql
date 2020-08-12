-- 提现出站 saga 行。自然键 transfer_id；幂等键 idem 防重复扣款。
-- 作者：Capson
-- 日期：2020.12.16

CREATE TABLE IF NOT EXISTS aa_wd_outbound (
  transfer_id VARCHAR(64) NOT NULL PRIMARY KEY,
  run_id VARCHAR(64) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL DEFAULT 0,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  idem VARCHAR(128) NOT NULL DEFAULT '',
  step VARCHAR(16) NOT NULL DEFAULT '',
  kyt VARCHAR(16) NOT NULL DEFAULT '',
  policy_ver INT NOT NULL DEFAULT 0,
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  debit_once TINYINT NOT NULL DEFAULT 0,
  fail_sealed TINYINT NOT NULL DEFAULT 0,
  compensate VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_wd_outbound_idem (idem),
  KEY idx_aa_wd_outbound_step (step, updated_at),
  KEY idx_aa_wd_outbound_user (user_id, asset, created_at)
);
