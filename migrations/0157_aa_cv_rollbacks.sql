-- 回滚计划表（cv_rollbacks）与穿仓事件表（cv_breaches）。
-- cv_rollbacks: 失败/撤单/过期回滚编排计划；状态机：planned→executing→done/failed。
-- cv_breaches:  穿仓事件（可用转负/双边不平衡/占用令牌漂移）；供运营板查询。
-- 作者：TianpengLan

CREATE TABLE IF NOT EXISTS cv_rollbacks (
  id            VARCHAR(64)  NOT NULL PRIMARY KEY,
  occ_id        VARCHAR(64)  NOT NULL,
  order_id      VARCHAR(64)  NOT NULL DEFAULT '',
  user_id       VARCHAR(64)  NOT NULL,
  trigger_type  VARCHAR(16)  NOT NULL DEFAULT 'CANCEL',
  status        VARCHAR(16)  NOT NULL DEFAULT 'PLANNED',
  filled_amount VARCHAR(64)  NOT NULL DEFAULT '0',
  remain_amount VARCHAR(64)  NOT NULL DEFAULT '0',
  idem_key      VARCHAR(128) NOT NULL DEFAULT '',
  error         VARCHAR(512) NOT NULL DEFAULT '',
  executed_at   DATETIME(3)           DEFAULT NULL,
  done_at       DATETIME(3)           DEFAULT NULL,
  created_at    DATETIME(3)  NOT NULL,
  updated_at    DATETIME(3)  NOT NULL,
  KEY idx_cv_rollbacks_occ    (occ_id),
  KEY idx_cv_rollbacks_status (status, created_at)
);

-- 回滚腿明细（内联存储于 cv_rollbacks 附属；如需独立查询可扩展为单独表）。
CREATE TABLE IF NOT EXISTS cv_rollback_legs (
  id         VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id    VARCHAR(64)  NOT NULL,
  action     VARCHAR(32)  NOT NULL DEFAULT '',
  ref        VARCHAR(256) NOT NULL DEFAULT '',
  amount     VARCHAR(64)  NOT NULL DEFAULT '',
  status     VARCHAR(16)  NOT NULL DEFAULT 'PENDING',
  error      VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME(3)  NOT NULL,
  updated_at DATETIME(3)  NOT NULL,
  KEY idx_cv_rollback_legs_plan (plan_id)
);

CREATE TABLE IF NOT EXISTS cv_breaches (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id      VARCHAR(64)  NOT NULL,
  occ_id       VARCHAR(64)  NOT NULL DEFAULT '',
  breach_kind  VARCHAR(32)  NOT NULL DEFAULT '',
  asset        VARCHAR(16)  NOT NULL DEFAULT '',
  occ_amount   VARCHAR(64)  NOT NULL DEFAULT '0',
  avail_before VARCHAR(64)  NOT NULL DEFAULT '0',
  avail_after  VARCHAR(64)  NOT NULL DEFAULT '0',
  detail       VARCHAR(512) NOT NULL DEFAULT '',
  resolved     TINYINT(1)   NOT NULL DEFAULT 0,
  resolved_at  DATETIME(3)           DEFAULT NULL,
  created_at   DATETIME(3)  NOT NULL,
  KEY idx_cv_breaches_user     (user_id, resolved, created_at),
  KEY idx_cv_breaches_kind     (breach_kind, resolved),
  KEY idx_cv_breaches_created  (created_at)
);
