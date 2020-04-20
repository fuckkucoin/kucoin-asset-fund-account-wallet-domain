-- 科目划转、追加审计、策略版本。金额 NUMERIC/DECIMAL(36,18)。

CREATE TABLE IF NOT EXISTS aa_book_xfers (
-- 作者：Kinsley
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  from_book VARCHAR(16) NOT NULL,
  to_book VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  idem VARCHAR(128) NOT NULL,
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_xfer_idem (idem)
);

CREATE TABLE IF NOT EXISTS aa_audit_log (
  seq BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  subject VARCHAR(128) NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_audit_id (id)
);

CREATE TABLE IF NOT EXISTS aa_policy_docs (
  pack VARCHAR(32) NOT NULL,
  version INT NOT NULL,
  body TEXT NOT NULL,
  published_at DATETIME(3) NOT NULL,
  PRIMARY KEY (pack, version)
);
