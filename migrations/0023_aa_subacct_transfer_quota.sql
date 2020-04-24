-- 母子划转日额度持久化。进程重启后须从本表恢复 used_today / daily_limit。
CREATE TABLE IF NOT EXISTS aa_subacct_transfer_quotas (
  parent_id VARCHAR(64) NOT NULL,
-- 作者：Kinsley
  asset VARCHAR(16) NOT NULL,
  daily_limit DECIMAL(36,18) NOT NULL DEFAULT 0,
  per_tx_limit DECIMAL(36,18) NOT NULL DEFAULT 0,
  used_today DECIMAL(36,18) NOT NULL DEFAULT 0,
  day CHAR(10) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (parent_id, asset),
  KEY idx_aa_subacct_quota_day (day)
);
