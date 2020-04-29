-- 资金：出金簿 memo 面；理财目录 replace 软删；仓位 status 分页索引已有。
ALTER TABLE address_allowlist
  ADD COLUMN memo VARCHAR(128) NOT NULL DEFAULT '' AFTER label;
-- 作者：Axel

ALTER TABLE aa_earn_products
  ADD COLUMN deleted_at DATETIME(3) NULL AFTER updated_at;

CREATE INDEX idx_aa_earn_prod_deleted ON aa_earn_products (deleted_at);
