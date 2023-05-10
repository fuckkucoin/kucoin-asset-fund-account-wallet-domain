-- 理财仓位 SQL 主存 + 产品目录/EarlyBPS 热更新落库；启动 Hydrate。
CREATE TABLE IF NOT EXISTS aa_earn_positions (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
-- 作者：Axel
  user_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  principal VARCHAR(64) NOT NULL DEFAULT '0',
  accrued VARCHAR(64) NOT NULL DEFAULT '0',
  claimed VARCHAR(64) NOT NULL DEFAULT '0',
  penalty VARCHAR(64) NOT NULL DEFAULT '',
  opened_at DATETIME(3) NOT NULL,
  unlock_at DATETIME(3) NULL,
  last_accrue DATETIME(3) NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'open',
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
  auto_subscribe TINYINT(1) NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_earn_pos_user (user_id, status),
  KEY idx_aa_earn_pos_idem (idempotency_key),
  KEY idx_aa_earn_pos_product (product_id, status)
);

CREATE TABLE IF NOT EXISTS aa_earn_products (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  asset VARCHAR(16) NOT NULL,
  category VARCHAR(32) NOT NULL DEFAULT 'flexible',
  daily_bps INT NOT NULL DEFAULT 0,
  lock_days INT NOT NULL DEFAULT 0,
  min_subscribe VARCHAR(64) NOT NULL DEFAULT '',
  max_position VARCHAR(64) NOT NULL DEFAULT '',
  early_bps INT NOT NULL DEFAULT 0,
  active TINYINT(1) NOT NULL DEFAULT 1,
  auto_subscribe TINYINT(1) NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_earn_prod_cat (category, active)
);
