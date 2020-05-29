-- 功能：归集冷却标记与执行竞态收口引擎——数据库表结构。
-- 重点：
--   sweep_cool_leases 记录每次 Mark(asset) 产生的冷却租约，state 按 FSM 单调流转。
--   sweep_cool_cases 为只增告警表，记录冷却窗内的非法执行企图与后验漂移事件。
--   sweep_cool_proofs 为只增证明表，记录冷却窗结束后的证明签发；禁止删改。
--   sweep_cool_generations 跟踪每个 asset 的世代号，防止同资产双开 Exec。
-- 风险点：
--   lease_id、case_id、proof_id 均为 UUID，VARCHAR(64) 足够；禁止缩短。
--   cool_until 用 DATETIME(3) UTC 存储，出库后需 .UTC() 转换。
--   resolved 列用 TINYINT(1) 存储布尔值（MySQL 无原生 BOOL 类型）。
--   所有时间列用 DATETIME(3) UTC 存储，不含时区后缀。
-- Author: TianpengLan
-- 作者：TianpengLan
-- Date: 2021-09-10

CREATE TABLE IF NOT EXISTS sweep_cool_leases (
  lease_id    VARCHAR(64)   NOT NULL,
  asset       VARCHAR(16)   NOT NULL,
  generation  BIGINT        NOT NULL DEFAULT 1,
  state       VARCHAR(16)   NOT NULL DEFAULT 'marked',
  marked_at   DATETIME(3)   NOT NULL,
  cool_until  DATETIME(3)   NOT NULL,
  cleared_at  DATETIME(3)   NULL,
  proof_id    VARCHAR(64)   NOT NULL DEFAULT '',
  reason      VARCHAR(255)  NOT NULL DEFAULT '',
  PRIMARY KEY (lease_id),
  KEY idx_scl_asset_state (asset, state),
  KEY idx_scl_cool_until (cool_until, state)
);

CREATE TABLE IF NOT EXISTS sweep_cool_cases (
  case_id      VARCHAR(64)   NOT NULL,
  asset        VARCHAR(16)   NOT NULL,
  generation   BIGINT        NOT NULL DEFAULT 0,
  lease_id     VARCHAR(64)   NOT NULL DEFAULT '',
  exec_id      VARCHAR(64)   NOT NULL DEFAULT '',
  attempted_at DATETIME(3)   NOT NULL,
  decision     VARCHAR(16)   NOT NULL DEFAULT 'blocked',
  reason       VARCHAR(512)  NOT NULL DEFAULT '',
  resolved     TINYINT(1)    NOT NULL DEFAULT 0,
  resolved_at  DATETIME(3)   NULL,
  PRIMARY KEY (case_id),
  KEY idx_scc_asset (asset, attempted_at),
  KEY idx_scc_resolved (resolved, attempted_at),
  KEY idx_scc_exec (exec_id)
);

CREATE TABLE IF NOT EXISTS sweep_cool_proofs (
  proof_id    VARCHAR(64)   NOT NULL,
  lease_id    VARCHAR(64)   NOT NULL,
  asset       VARCHAR(16)   NOT NULL,
  generation  BIGINT        NOT NULL DEFAULT 0,
  cool_until  DATETIME(3)   NOT NULL,
  signed_at   DATETIME(3)   NOT NULL,
  digest      VARCHAR(64)   NOT NULL DEFAULT '',
  PRIMARY KEY (proof_id),
  UNIQUE KEY uk_scp_lease (lease_id),
  KEY idx_scp_asset_gen (asset, generation),
  KEY idx_scp_signed (asset, signed_at)
);

CREATE TABLE IF NOT EXISTS sweep_cool_generations (
  gen_id           VARCHAR(64)   NOT NULL,
  asset            VARCHAR(16)   NOT NULL,
  generation       BIGINT        NOT NULL DEFAULT 0,
  active_lease_id  VARCHAR(64)   NOT NULL DEFAULT '',
  updated_at       DATETIME(3)   NOT NULL,
  PRIMARY KEY (gen_id),
  UNIQUE KEY uk_scg_asset (asset)
);
