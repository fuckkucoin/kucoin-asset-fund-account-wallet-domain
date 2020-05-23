-- UTA 杠杆借还读面：利率阶梯、负债、抵押折扣、还本流水、风险快照、全仓科目。
-- 写路径（借入/强平）在账户邻域；本表仅资金 REST 读面权威源。

-- 作者：Kinsley
CREATE TABLE IF NOT EXISTS aa_uta_rate_ladder (
  currency VARCHAR(16) NOT NULL,
  tier INT NOT NULL DEFAULT 0,
  rate_hourly VARCHAR(32) NOT NULL DEFAULT '0',
  rate_daily VARCHAR(32) NOT NULL DEFAULT '0',
  borrow_limit VARCHAR(48) NOT NULL DEFAULT '0',
  interest_free_limit VARCHAR(48) NOT NULL DEFAULT '0',
  hold_limit VARCHAR(48) NOT NULL DEFAULT '0',
  updated_at BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (currency, tier)
);

CREATE TABLE IF NOT EXISTS aa_uta_liability (
  user_id VARCHAR(64) NOT NULL,
  currency VARCHAR(16) NOT NULL,
  margin_mode VARCHAR(16) NOT NULL DEFAULT 'CROSS',
  symbol VARCHAR(32) NOT NULL DEFAULT '',
  principal VARCHAR(48) NOT NULL DEFAULT '0',
  interest VARCHAR(48) NOT NULL DEFAULT '0',
  accrued_interest VARCHAR(48) NOT NULL DEFAULT '0',
  borrow_enabled TINYINT NOT NULL DEFAULT 1,
  updated_at BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, currency, margin_mode, symbol),
  KEY idx_aa_uta_liab_user (user_id, margin_mode)
);

CREATE TABLE IF NOT EXISTS aa_uta_collateral (
  currency VARCHAR(16) NOT NULL PRIMARY KEY,
  haircut VARCHAR(16) NOT NULL DEFAULT '1',
  collateral_enabled TINYINT NOT NULL DEFAULT 1,
  max_ltv VARCHAR(16) NOT NULL DEFAULT '0.85',
  updated_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS aa_uta_repay_history (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  currency VARCHAR(16) NOT NULL,
  margin_mode VARCHAR(16) NOT NULL DEFAULT 'CROSS',
  symbol VARCHAR(32) NOT NULL DEFAULT '',
  repay_type VARCHAR(16) NOT NULL DEFAULT 'REPAY',
  principal VARCHAR(48) NOT NULL DEFAULT '0',
  interest VARCHAR(48) NOT NULL DEFAULT '0',
  created_at BIGINT NOT NULL,
  KEY idx_aa_uta_repay_user (user_id, currency, created_at)
);

CREATE TABLE IF NOT EXISTS aa_uta_risk_snapshot (
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  margin_level VARCHAR(32) NOT NULL DEFAULT '0',
  total_equity VARCHAR(48) NOT NULL DEFAULT '0',
  total_liability VARCHAR(48) NOT NULL DEFAULT '0',
  total_collateral VARCHAR(48) NOT NULL DEFAULT '0',
  risk_grade VARCHAR(8) NOT NULL DEFAULT 'LOW',
  cross_debt_ratio VARCHAR(32) NOT NULL DEFAULT '0',
  updated_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS aa_uta_cross_margin (
  user_id VARCHAR(64) NOT NULL,
  currency VARCHAR(16) NOT NULL,
  total VARCHAR(48) NOT NULL DEFAULT '0',
  available VARCHAR(48) NOT NULL DEFAULT '0',
  hold VARCHAR(48) NOT NULL DEFAULT '0',
  liability VARCHAR(48) NOT NULL DEFAULT '0',
  max_borrow VARCHAR(48) NOT NULL DEFAULT '0',
  borrow_enabled TINYINT NOT NULL DEFAULT 1,
  transfer_enabled TINYINT NOT NULL DEFAULT 1,
  updated_at BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, currency)
);
