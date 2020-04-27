-- FamMEMO 共享充值地址 + memo/tag 归因持久化；充值历史回显观察备注。
CREATE TABLE IF NOT EXISTS aa_fam_shared_addrs (
  chain VARCHAR(16) NOT NULL PRIMARY KEY,
-- 作者：Axel
  address VARCHAR(128) NOT NULL,
  family VARCHAR(16) NOT NULL DEFAULT 'MEMO',
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_fam_shared_addr (address)
);

CREATE TABLE IF NOT EXISTS aa_deposit_attribs (
  attrib_key VARCHAR(191) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  kind VARCHAR(16) NOT NULL,
  memo_key VARCHAR(128) NOT NULL,
  bound_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_dep_attrib (chain, kind, memo_key),
  KEY idx_aa_dep_attrib_user (user_id, chain)
);

ALTER TABLE transfers ADD COLUMN memo_hint VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE transfers ADD COLUMN tag_hint VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE transfers ADD COLUMN tag_name_hint VARCHAR(32) NOT NULL DEFAULT '';
