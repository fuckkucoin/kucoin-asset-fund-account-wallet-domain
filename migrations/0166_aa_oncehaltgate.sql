-- 出箱一次性停机投递闸：停机世代租约、双投/漏投证明仓、在途 Deliver 吊销日志。
--
-- 设计原则：
--   1. outboxonce_halt_leases：每次 SetHalt 切换建一行（同世代幂等），记录世代、状态、吊销数量。
--   2. outboxonce_halt_cases：双投/漏投证明，只追加，不修改；halt_gen 单调递增。
--   3. outboxonce_deliver_gens：Deliver 绑定世代快照（在途追踪），吊销后 revoked=1。
--
-- 作者：TracyWong
-- 日期：2024.06.08

-- ──────────────────────────────────────────────────────────────────────────────
-- 停机世代租约
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS outboxonce_halt_leases (
  id              VARCHAR(64)   NOT NULL,
  generation      BIGINT        NOT NULL,
  state           VARCHAR(16)   NOT NULL DEFAULT 'cleared',
  set_by          VARCHAR(64)   NOT NULL DEFAULT '',
  revoked_count   INT           NOT NULL DEFAULT 0,
  set_at          DATETIME(3)   NOT NULL,
  cleared_at      DATETIME(3)            DEFAULT NULL,
  created_at      DATETIME(3)   NOT NULL,
  updated_at      DATETIME(3)   NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_halt_leases_gen (generation),
  KEY idx_halt_leases_state (state, set_at),
  KEY idx_halt_leases_set_by (set_by, set_at)
);

-- ──────────────────────────────────────────────────────────────────────────────
-- 双投/漏投证明仓（只追加）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS outboxonce_halt_cases (
  id              VARCHAR(64)   NOT NULL,
  kind            VARCHAR(32)   NOT NULL,
  event_key       VARCHAR(191)  NOT NULL,
  event_id        VARCHAR(64)   NOT NULL,
  halt_gen        BIGINT        NOT NULL,
  epoch           BIGINT        NOT NULL DEFAULT 0,
  token           VARCHAR(64)   NOT NULL DEFAULT '',
  conflict_id     VARCHAR(64)   NOT NULL DEFAULT '',
  evidence        VARCHAR(2048) NOT NULL DEFAULT '',
  detected_at     DATETIME(3)   NOT NULL,
  exported_at     DATETIME(3)            DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_halt_cases_key (event_key, detected_at),
  KEY idx_halt_cases_kind (kind, detected_at),
  KEY idx_halt_cases_gen (halt_gen, detected_at),
  KEY idx_halt_cases_event (event_id, detected_at)
);

-- ──────────────────────────────────────────────────────────────────────────────
-- 在途 Deliver 绑定世代快照（吊销追踪）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS outboxonce_deliver_gens (
  id              VARCHAR(64)   NOT NULL,
  event_key       VARCHAR(191)  NOT NULL,
  event_id        VARCHAR(64)   NOT NULL,
  halt_gen        BIGINT        NOT NULL,
  epoch           BIGINT        NOT NULL,
  token           VARCHAR(64)   NOT NULL,
  revoked         TINYINT(1)    NOT NULL DEFAULT 0,
  locked_at       DATETIME(3)   NOT NULL,
  revoked_at      DATETIME(3)            DEFAULT NULL,
  committed_at    DATETIME(3)            DEFAULT NULL,
  created_at      DATETIME(3)   NOT NULL,
  PRIMARY KEY (id),
  KEY idx_deliver_gens_key (event_key, locked_at),
  KEY idx_deliver_gens_gen (halt_gen, revoked),
  KEY idx_deliver_gens_revoked (revoked, locked_at)
);
