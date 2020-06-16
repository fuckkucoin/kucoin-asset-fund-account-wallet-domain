-- 功能：库务计划执行租约表与漂移证明表。
-- 重点：
--   treasury_plan_leases 绑定 SweepPlan ExecID 与热库存快照，
--   TTL 到期后由 worker 扫描标记 expired，禁止手工删行。
--   treasury_exec_drift_proofs 为只增表，禁止删改；存储漂移拒绝证明。
--   treasury_exec_generations 跟踪同 plan 的世代，防止双开 Exec。
-- 风险点：
--   lease_id 与 exec_id 均为 UUID，VARCHAR(64) 足够；禁止缩短。
--   snapshot_json 存储 SnapshotDigest JSON，长度按实测预留 2048。
--   所有时间列用 DATETIME(3) UTC 存储，不含时区后缀。
-- Author: TianpengLan
-- 作者：Kinsley
-- Date: 2021-06-15

CREATE TABLE IF NOT EXISTS treasury_plan_leases (
  lease_id         VARCHAR(64)    NOT NULL,
  idem_key         VARCHAR(64)    NOT NULL,
  exec_id          VARCHAR(64)    NOT NULL DEFAULT '',
  asset            VARCHAR(16)    NOT NULL,
  amount           VARCHAR(64)    NOT NULL,
  snapshot_json    TEXT           NOT NULL,
  state            VARCHAR(16)    NOT NULL DEFAULT 'bound',
  ttl_seconds      BIGINT         NOT NULL DEFAULT 600,
  bound_at         DATETIME(3)    NOT NULL,
  expires_at       DATETIME(3)    NOT NULL,
  recheck_at       DATETIME(3)    NULL,
  sent_at          DATETIME(3)    NULL,
  acked_at         DATETIME(3)    NULL,
  blocked_at       DATETIME(3)    NULL,
  block_reason     VARCHAR(255)   NOT NULL DEFAULT '',
  PRIMARY KEY (lease_id),
  UNIQUE KEY uk_tpl_idem (idem_key, state),
  KEY idx_tpl_exec (exec_id),
  KEY idx_tpl_expires (expires_at, state),
  KEY idx_tpl_state (state, bound_at)
);

CREATE TABLE IF NOT EXISTS treasury_exec_drift_proofs (
  proof_id               VARCHAR(64)    NOT NULL,
  lease_id               VARCHAR(64)    NOT NULL,
  exec_id                VARCHAR(64)    NOT NULL DEFAULT '',
  asset                  VARCHAR(16)    NOT NULL,
  record_snapshot_json   TEXT           NOT NULL,
  recheck_snapshot_json  TEXT           NOT NULL,
  drift_abs              VARCHAR(64)    NOT NULL DEFAULT '0',
  drift_ratio            VARCHAR(64)    NOT NULL DEFAULT '0',
  decision               VARCHAR(16)    NOT NULL DEFAULT 'blocked',
  reason                 VARCHAR(512)   NOT NULL DEFAULT '',
  created_at             DATETIME(3)    NOT NULL,
  PRIMARY KEY (proof_id),
  KEY idx_tedp_lease (lease_id, created_at),
  KEY idx_tedp_exec (exec_id, created_at)
);

CREATE TABLE IF NOT EXISTS treasury_exec_generations (
  gen_id           VARCHAR(64)    NOT NULL,
  idem_key         VARCHAR(64)    NOT NULL,
  generation       BIGINT         NOT NULL DEFAULT 1,
  active_lease_id  VARCHAR(64)    NOT NULL DEFAULT '',
  updated_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (gen_id),
  UNIQUE KEY uk_teg_idem (idem_key)
);
