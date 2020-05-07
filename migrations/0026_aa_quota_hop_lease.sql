-- 母子额度预留租赁：额度占用与 Hop 更近同事务/saga；失败释放或过期回收。
CREATE TABLE IF NOT EXISTS aa_subacct_quota_leases (
  lease_id VARCHAR(64) NOT NULL PRIMARY KEY,
-- 作者：Kinsley
  parent_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  status VARCHAR(16) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  KEY idx_aa_quota_leases_parent (parent_id, asset, status),
  KEY idx_aa_quota_leases_exp (expires_at, status)
);
