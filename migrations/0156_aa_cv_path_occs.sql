-- 闪兑路径占用会计主表（cv_path_occs）。
-- 每条限价/市价路径对应一行；状态机：quoting→frozen→partial→settled/rolled_back。
-- 不存储出金广播字段；不存储出金失败补偿编排字段。
-- 作者：Kinsley
CREATE TABLE IF NOT EXISTS cv_path_occs (
  id            VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id       VARCHAR(64)  NOT NULL,
  order_id      VARCHAR(64)  NOT NULL DEFAULT '',
  client_oid    VARCHAR(128) NOT NULL DEFAULT '',
  path_kind     VARCHAR(16)  NOT NULL DEFAULT 'LIMIT',
  from_asset    VARCHAR(16)  NOT NULL DEFAULT '',
  to_asset      VARCHAR(16)  NOT NULL DEFAULT '',
  total_amount  VARCHAR(64)  NOT NULL DEFAULT '0',
  filled_amount VARCHAR(64)  NOT NULL DEFAULT '0',
  remain_amount VARCHAR(64)  NOT NULL DEFAULT '0',
  status        VARCHAR(16)  NOT NULL DEFAULT 'QUOTING',
  hold_token    VARCHAR(128) NOT NULL DEFAULT '',
  quote_id      VARCHAR(64)  NOT NULL DEFAULT '',
  account_type  VARCHAR(16)  NOT NULL DEFAULT 'MAIN',
  rollback_id   VARCHAR(64)  NOT NULL DEFAULT '',
  breach_id     VARCHAR(64)  NOT NULL DEFAULT '',
  expire_at     DATETIME(3)  NOT NULL,
  frozen_at     DATETIME(3)           DEFAULT NULL,
  settled_at    DATETIME(3)           DEFAULT NULL,
  rolled_at     DATETIME(3)           DEFAULT NULL,
  created_at    DATETIME(3)  NOT NULL,
  updated_at    DATETIME(3)  NOT NULL,
  KEY idx_cv_path_occs_user   (user_id, status, updated_at),
  KEY idx_cv_path_occs_order  (order_id),
  KEY idx_cv_path_occs_expire (expire_at, status)
);

-- 占用腿明细（cv_occ_legs）。
-- 每条路径至少有一条源资产腿；部分成交时腿的 released 累加。
CREATE TABLE IF NOT EXISTS cv_occ_legs (
  id         VARCHAR(64) NOT NULL PRIMARY KEY,
  occ_id     VARCHAR(64) NOT NULL,
  user_id    VARCHAR(64) NOT NULL,
  leg_kind   VARCHAR(16) NOT NULL DEFAULT 'SOURCE',
  asset      VARCHAR(16) NOT NULL DEFAULT '',
  amount     VARCHAR(64) NOT NULL DEFAULT '0',
  released   VARCHAR(64) NOT NULL DEFAULT '0',
  hold_ref   VARCHAR(128) NOT NULL DEFAULT '',
  fill_seq   VARCHAR(64)  NOT NULL DEFAULT '',
  closed     TINYINT(1)   NOT NULL DEFAULT 0,
  created_at DATETIME(3)  NOT NULL,
  updated_at DATETIME(3)  NOT NULL,
  KEY idx_cv_occ_legs_occ (occ_id),
  KEY idx_cv_occ_legs_user (user_id, asset, closed)
);
