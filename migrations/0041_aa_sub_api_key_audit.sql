-- 子账户 API Key 审计流水与配额闸。以 SQL 为权威。

CREATE TABLE IF NOT EXISTS aa_sub_api_key_audit (
-- 作者：TianpengLan
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  owner VARCHAR(64) NOT NULL,
  sub_name VARCHAR(64) NOT NULL DEFAULT '',
  api_key VARCHAR(64) NOT NULL DEFAULT '',
  action VARCHAR(32) NOT NULL,
  detail VARCHAR(255) NOT NULL DEFAULT '',
  client_ip VARCHAR(64) NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  KEY idx_aa_sub_key_audit_owner (owner, created_at),
  KEY idx_aa_sub_key_audit_key (api_key, created_at)
);

CREATE TABLE IF NOT EXISTS aa_sub_key_quota (
  owner VARCHAR(64) NOT NULL,
  sub_name VARCHAR(64) NOT NULL DEFAULT '',
  max_keys INT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (owner, sub_name)
);

ALTER TABLE aa_sub_api_keys ADD COLUMN state VARCHAR(16) NOT NULL DEFAULT 'active';
ALTER TABLE aa_sub_api_keys ADD COLUMN rotated_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE aa_sub_api_keys ADD COLUMN revoked_at BIGINT NOT NULL DEFAULT 0;
