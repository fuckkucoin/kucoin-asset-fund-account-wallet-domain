-- 功能：科目树版本存储与会计政策引擎持久化表。
-- 边界：这些表不写账本分录，不替代 ledger 状态机；subjtree_version 存节点/边快照，
--       acctpol_version 存政策规则快照，acctpol_audit 只追加不删改。
-- 作者：Axel
-- 不变量：subjtree_version.version 唯一递增；acctpol_version.version 唯一递增；
--         acctpol_audit 无主键以外的唯一约束，允许同一版本出现多条不同操作记录。

CREATE TABLE IF NOT EXISTS subjtree_version (
  version       INT NOT NULL PRIMARY KEY,
  published_at  DATETIME(3) NOT NULL,
  author        VARCHAR(128) NOT NULL DEFAULT '',
  memo          VARCHAR(512) NOT NULL DEFAULT '',
  -- payload 存储完整的 TreeVersion JSON（含 nodes/edges 数组）
  payload       MEDIUMTEXT NOT NULL,
  KEY idx_subjtree_published (published_at)
);

CREATE TABLE IF NOT EXISTS acctpol_version (
  version       INT NOT NULL PRIMARY KEY,
  published_at  DATETIME(3) NOT NULL,
  author        VARCHAR(128) NOT NULL DEFAULT '',
  memo          VARCHAR(512) NOT NULL DEFAULT '',
  -- current_flag=1 表示当前活跃版本（回滚时更新）；最多一行为 1
  current_flag  TINYINT NOT NULL DEFAULT 0,
  -- payload 存储完整的 PolicyVersion JSON（含 rules 数组）
  payload       MEDIUMTEXT NOT NULL,
  KEY idx_acctpol_current (current_flag),
  KEY idx_acctpol_published (published_at)
);

CREATE TABLE IF NOT EXISTS acctpol_audit (
  id            VARCHAR(64) NOT NULL PRIMARY KEY,
  at            DATETIME(3) NOT NULL,
  action        VARCHAR(32) NOT NULL,
  actor         VARCHAR(128) NOT NULL,
  from_version  INT NULL,
  to_version    INT NULL,
  -- subject 用于 pin_user/unpin_user 时记录目标 user_id
  subject       VARCHAR(128) NULL,
  reason        VARCHAR(512) NULL,
  KEY idx_acctpol_audit_at (at),
  KEY idx_acctpol_audit_action (action, at)
);
