-- 资金偏好与默认账户。
-- 按户落默认科目、展示币、隐藏零余额、资产列表序；变更追加审计。KV 热缓存 kind 为 fundpref.row / fundpref.audit，与 fundsurf 冻结键隔离。body 损坏不得当默认；审计只追加。主写 SG 10.18 / TYO 10.19 只读回放。
CREATE TABLE IF NOT EXISTS aa_fund_pref (
-- 作者：TianpengLan
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  body BLOB NOT NULL,
  version BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_fund_pref_audit (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  action VARCHAR(16) NOT NULL,
  before_body TEXT NOT NULL,
  after_body TEXT NOT NULL,
  at DATETIME(3) NOT NULL,
  KEY idx_aa_fund_pref_audit (user_id, at)
);
