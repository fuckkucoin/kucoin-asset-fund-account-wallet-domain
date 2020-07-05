-- 迁移 0054：forbid_gate 禁提世代与出金闸竞态封堵表集
-- 涵盖：fundsurf_forbid_leases, fundsurf_wd_binds,
--       fundsurf_forbid_drift_cases, fundsurf_forbid_drift_proofs
-- 作者：Renne  日期：2020-12-02

-- fundsurf_forbid_leases：禁提世代租约
-- 每次 SetForbidden 切换均写一行（generation 递增）；最新 generation 为当前有效世代。
CREATE TABLE IF NOT EXISTS fundsurf_forbid_leases (
  id           VARCHAR(64)   NOT NULL PRIMARY KEY,
  user_id      VARCHAR(64)   NOT NULL,
  generation   BIGINT        NOT NULL DEFAULT 1,
  state        VARCHAR(8)    NOT NULL DEFAULT 'off',
  actor        VARCHAR(64)   NOT NULL DEFAULT '',
  reason       VARCHAR(255)  NOT NULL DEFAULT '',
  proof_id     VARCHAR(64)   NOT NULL DEFAULT '',
  issued_at    DATETIME(3)   NOT NULL,
  switched_at  DATETIME(3)   NULL,
  revoked_at   DATETIME(3)   NULL,
  fence_epoch  BIGINT        NOT NULL DEFAULT 0,
  fence_token  VARCHAR(64)   NOT NULL DEFAULT '',
  UNIQUE KEY uk_fg_lease_user_gen (user_id, generation),
  KEY idx_fg_lease_user (user_id, issued_at)
);

-- fundsurf_wd_binds：出金预检绑定
-- RequireWithdraw 通过后写入；SetForbidden(true) 触发 revoke；广播前 consume。
CREATE TABLE IF NOT EXISTS fundsurf_wd_binds (
  id              VARCHAR(64)   NOT NULL PRIMARY KEY,
  user_id         VARCHAR(64)   NOT NULL,
  withdraw_ref    VARCHAR(128)  NOT NULL DEFAULT '',
  generation      BIGINT        NOT NULL DEFAULT 0,
  forbid_lease_id VARCHAR(64)   NOT NULL DEFAULT '',
  state           VARCHAR(16)   NOT NULL DEFAULT 'open',
  issued_at       DATETIME(3)   NOT NULL,
  expires_at      DATETIME(3)   NOT NULL,
  revoked_at      DATETIME(3)   NULL,
  revoke_reason   VARCHAR(255)  NOT NULL DEFAULT '',
  consumed_at     DATETIME(3)   NULL,
  fence_epoch     BIGINT        NOT NULL DEFAULT 0,
  fence_token     VARCHAR(64)   NOT NULL DEFAULT '',
  KEY idx_fg_wd_binds_user_state (user_id, state, issued_at),
  KEY idx_fg_wd_binds_ref (withdraw_ref),
  KEY idx_fg_wd_binds_gen (user_id, generation, state),
  KEY idx_fg_wd_binds_expires (state, expires_at)
);

-- fundsurf_forbid_drift_cases：禁提漂移案例
-- 世代不符（预检时世代 ≠ 广播前世代）时写入；解除挡写须证明通过。
CREATE TABLE IF NOT EXISTS fundsurf_forbid_drift_cases (
  id              VARCHAR(64)   NOT NULL PRIMARY KEY,
  user_id         VARCHAR(64)   NOT NULL,
  withdraw_ref    VARCHAR(128)  NOT NULL DEFAULT '',
  bind_id         VARCHAR(64)   NOT NULL DEFAULT '',
  forbid_lease_id VARCHAR(64)   NOT NULL DEFAULT '',
  kind            VARCHAR(32)   NOT NULL DEFAULT '',
  gen_at_precheck BIGINT        NOT NULL DEFAULT 0,
  gen_at_detect   BIGINT        NOT NULL DEFAULT 0,
  state           VARCHAR(16)   NOT NULL DEFAULT 'open',
  detected_at     DATETIME(3)   NOT NULL,
  proven_at       DATETIME(3)   NULL,
  closed_at       DATETIME(3)   NULL,
  proof_id        VARCHAR(64)   NOT NULL DEFAULT '',
  proof_summary   VARCHAR(512)  NOT NULL DEFAULT '',
  closed_by       VARCHAR(64)   NOT NULL DEFAULT '',
  fence_epoch     BIGINT        NOT NULL DEFAULT 0,
  fence_token     VARCHAR(64)   NOT NULL DEFAULT '',
  KEY idx_fg_drift_user_state (user_id, state, detected_at),
  KEY idx_fg_drift_bind (bind_id),
  KEY idx_fg_drift_ref (withdraw_ref)
);

-- fundsurf_forbid_drift_proofs：漂移证明（双人确认）
-- 含 user/actor/reason/gen 字段路径（通过关联 lease）；缺失证明不得解除禁提挡写。
CREATE TABLE IF NOT EXISTS fundsurf_forbid_drift_proofs (
  id            VARCHAR(64)   NOT NULL PRIMARY KEY,
  drift_case_id VARCHAR(64)   NOT NULL,
  submitted_by  VARCHAR(64)   NOT NULL DEFAULT '',
  confirmed_by  VARCHAR(64)   NOT NULL DEFAULT '',
  summary       VARCHAR(512)  NOT NULL DEFAULT '',
  evidence      TEXT          NOT NULL,
  submitted_at  DATETIME(3)   NOT NULL,
  confirmed_at  DATETIME(3)   NULL,
  KEY idx_fg_drift_proofs_case (drift_case_id, submitted_at)
);
