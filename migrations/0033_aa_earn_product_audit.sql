-- 理财目录软删审计：谁删/谁恢复独立表，与 aa_earn_products.deleted_at 解耦。
CREATE TABLE IF NOT EXISTS aa_earn_product_audit (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
-- 作者：Kinsley
  product_id VARCHAR(64) NOT NULL,
  action VARCHAR(16) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_earn_prod_audit_prod (product_id, at),
  KEY idx_aa_earn_prod_audit_at (at)
);
