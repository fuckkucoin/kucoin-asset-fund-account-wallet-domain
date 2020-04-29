-- 理财目录审计冷存清单：按产品分片归档后留索引，SQL 热表可 purge。
CREATE TABLE IF NOT EXISTS aa_earn_product_audit_cold (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
-- 作者：Axel
  archive_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL DEFAULT '',
  path VARCHAR(512) NOT NULL,
  row_count INT NOT NULL DEFAULT 0,
  start_at DATETIME(3) NOT NULL,
  end_at DATETIME(3) NOT NULL,
  sha256 CHAR(64) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_earn_audit_cold_id (archive_id),
  KEY idx_aa_earn_audit_cold_prod (product_id, at),
  KEY idx_aa_earn_audit_cold_at (at)
);
