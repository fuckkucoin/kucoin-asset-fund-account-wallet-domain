-- 资金配置灰度双写与策略脑裂封堵——数据库迁移（第 48 版）。
--
-- 新增五张表：
--   gray_releases       GrayRelease 灰度发布聚合根
--   dual_decisions      DualDecision 双评决策记录
--   split_diffs         SplitDiff 差分聚合记录
--   cfg_seal_actions    SealAction 封堵闸动作审计
--   cfg_rollback_proofs RollbackProof 回滚证明
--
-- 隔离约束：
--   本迁移不写入、不修改 subjectpol_* 表及 r5/r4 相关表。
--   gray_release_id / split_diff_id 作为只读外键供 A–G 其他包引用。
--
-- 作者：TateVoss
-- 日期：2020.12.12

CREATE TABLE IF NOT EXISTS gray_releases (
  id               VARCHAR(64)  NOT NULL PRIMARY KEY,
  pack_name        VARCHAR(128) NOT NULL,
  baseline_version INT          NOT NULL,
  canary_version   INT          NOT NULL,
  state            VARCHAR(32)  NOT NULL DEFAULT 'draft',
  cohort_percent   INT          NOT NULL DEFAULT 0,
  generation       INT          NOT NULL DEFAULT 1,
  policy_allow_drift INT        NOT NULL DEFAULT 20,
  creator          VARCHAR(64)  NOT NULL DEFAULT '',
  created_at       DATETIME(3)  NOT NULL,
  updated_at       DATETIME(3)  NOT NULL,
  sealed_at        DATETIME(3)  NULL,
  rolled_back_at   DATETIME(3)  NULL,
  KEY idx_gray_releases_pack_state (pack_name, state),
  KEY idx_gray_releases_state      (state)
);

CREATE TABLE IF NOT EXISTS dual_decisions (
  id               VARCHAR(64)  NOT NULL PRIMARY KEY,
  gray_release_id  VARCHAR(64)  NOT NULL,
  generation       INT          NOT NULL DEFAULT 1,
  user_id          VARCHAR(64)  NOT NULL,
  asset            VARCHAR(16)  NOT NULL DEFAULT '',
  amount           VARCHAR(64)  NOT NULL DEFAULT '',
  pack_name        VARCHAR(128) NOT NULL,
  baseline_allow   TINYINT(1)   NOT NULL DEFAULT 0,
  baseline_reason  VARCHAR(255) NOT NULL DEFAULT '',
  canary_allow     TINYINT(1)   NOT NULL DEFAULT 0,
  canary_reason    VARCHAR(255) NOT NULL DEFAULT '',
  has_diff         TINYINT(1)   NOT NULL DEFAULT 0,
  diff_kind        VARCHAR(32)  NOT NULL DEFAULT '',
  split_diff_id    VARCHAR(64)  NOT NULL DEFAULT '',
  applied          VARCHAR(16)  NOT NULL DEFAULT 'baseline',
  sealed           TINYINT(1)   NOT NULL DEFAULT 0,
  created_at       DATETIME(3)  NOT NULL,
  KEY idx_dual_decisions_release_gen  (gray_release_id, generation),
  KEY idx_dual_decisions_release_diff (gray_release_id, has_diff),
  KEY idx_dual_decisions_created_at   (created_at)
);

CREATE TABLE IF NOT EXISTS split_diffs (
  id               VARCHAR(64)  NOT NULL PRIMARY KEY,
  gray_release_id  VARCHAR(64)  NOT NULL,
  generation       INT          NOT NULL DEFAULT 1,
  window_start     DATETIME(3)  NOT NULL,
  window_end       DATETIME(3)  NOT NULL,
  total_decisions  BIGINT       NOT NULL DEFAULT 0,
  diff_count       BIGINT       NOT NULL DEFAULT 0,
  diff_ratio       DOUBLE       NOT NULL DEFAULT 0,
  allow_deny_count BIGINT       NOT NULL DEFAULT 0,
  deny_allow_count BIGINT       NOT NULL DEFAULT 0,
  limit_drift_count BIGINT      NOT NULL DEFAULT 0,
  kill_switch_count BIGINT      NOT NULL DEFAULT 0,
  exceeds_policy   TINYINT(1)   NOT NULL DEFAULT 0,
  created_at       DATETIME(3)  NOT NULL,
  KEY idx_split_diffs_release       (gray_release_id, created_at),
  KEY idx_split_diffs_exceeds       (gray_release_id, exceeds_policy)
);

CREATE TABLE IF NOT EXISTS cfg_seal_actions (
  id               VARCHAR(64)  NOT NULL PRIMARY KEY,
  gray_release_id  VARCHAR(64)  NOT NULL,
  split_diff_id    VARCHAR(64)  NOT NULL DEFAULT '',
  from_state       VARCHAR(32)  NOT NULL DEFAULT '',
  to_state         VARCHAR(32)  NOT NULL,
  reason           VARCHAR(512) NOT NULL DEFAULT '',
  actor            VARCHAR(64)  NOT NULL DEFAULT 'system',
  created_at       DATETIME(3)  NOT NULL,
  KEY idx_cfg_seal_actions_release (gray_release_id, created_at)
);

CREATE TABLE IF NOT EXISTS cfg_rollback_proofs (
  id                   VARCHAR(64)  NOT NULL PRIMARY KEY,
  gray_release_id      VARCHAR(64)  NOT NULL,
  pack_name            VARCHAR(128) NOT NULL,
  rolled_back_version  INT          NOT NULL,
  restored_version     INT          NOT NULL,
  generation_before    INT          NOT NULL,
  generation_after     INT          NOT NULL,
  trigger_split_diff_id VARCHAR(64) NOT NULL DEFAULT '',
  actor                VARCHAR(64)  NOT NULL DEFAULT 'system',
  reason               VARCHAR(512) NOT NULL DEFAULT '',
  created_at           DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_cfg_rollback_proofs_release (gray_release_id),
  KEY idx_cfg_rollback_proofs_created_at (created_at)
);
