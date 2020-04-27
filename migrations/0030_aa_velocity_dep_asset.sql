-- 母子划转频次多实例命中 + 入金异常台 Asset 字段。
CREATE TABLE IF NOT EXISTS aa_subacct_velocity_gate (
  owner_id VARCHAR(64) NOT NULL PRIMARY KEY,
-- 作者：TianpengLan
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_subacct_velocity_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  owner_id VARCHAR(64) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  hit_at DATETIME(3) NOT NULL,
  KEY idx_aa_subacct_vel_owner_at (owner_id, hit_at)
);

ALTER TABLE aa_dep_tickets ADD COLUMN asset VARCHAR(16) NOT NULL DEFAULT '';
