-- 入金贷记出箱投递围栏世代：投递租约、事故证明、投递证明三表。
--
-- 设计原则：
--   1. fund_credit_deliver_leases：每次 EnqueueCredit 创建一行，绑定 board/lane 世代指纹；
--      DeliverCredit 复验世代后更新状态至 delivered；护栏翻转后更新至 revoked。
--   2. fund_credit_fence_cases：事故证明，只追加，不修改；世代漂移/双贷记/围栏关闭均在此落仓。
--   3. fund_credit_proofs：投递证明，DeliverCredit 成功后追加一行，世代与租约一致。
--
-- 与前轮隔离：
--   - 不共享 outboxonce_* 表（r15 F outboxonce Halt↔Deliver 已独立）；
--   - 不修改 fund_safety_* 核心表；
--   - credit_deliver_gen 字段仅在本三表中使用，供外部只读消费。
--
-- 作者：TalynMoss
-- 日期：2020.12.05

-- ──────────────────────────────────────────────────────────────────────────────
-- 板卡世代快照（持久化，用于跨进程世代同步）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fund_credit_board_gens (
  id              VARCHAR(64)   NOT NULL,
  board_key       VARCHAR(191)  NOT NULL,
  world_gen       BIGINT        NOT NULL DEFAULT 1,
  lane_gen        BIGINT        NOT NULL DEFAULT 1,
  closed          TINYINT(1)    NOT NULL DEFAULT 0,
  closed_at       DATETIME(3)            DEFAULT NULL,
  updated_at      DATETIME(3)   NOT NULL,
  created_at      DATETIME(3)   NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_credit_board_gens_key (board_key),
  KEY idx_credit_board_gens_closed (closed, updated_at),
  KEY idx_credit_board_gens_updated (updated_at)
);

-- ──────────────────────────────────────────────────────────────────────────────
-- 贷记投递租约
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fund_credit_deliver_leases (
  id              VARCHAR(64)   NOT NULL,
  credit_key      VARCHAR(191)  NOT NULL,
  board_key       VARCHAR(191)  NOT NULL,
  bind_world_gen  BIGINT        NOT NULL,
  bind_lane_gen   BIGINT        NOT NULL,
  epoch           BIGINT        NOT NULL,
  token           VARCHAR(64)   NOT NULL,
  amount          VARCHAR(64)   NOT NULL DEFAULT '',
  state           VARCHAR(16)   NOT NULL DEFAULT 'enqueued',
  enqueued_at     DATETIME(3)   NOT NULL,
  delivering_at   DATETIME(3)            DEFAULT NULL,
  delivered_at    DATETIME(3)            DEFAULT NULL,
  revoked_at      DATETIME(3)            DEFAULT NULL,
  revoke_reason   VARCHAR(512)  NOT NULL DEFAULT '',
  created_at      DATETIME(3)   NOT NULL,
  updated_at      DATETIME(3)   NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_credit_leases_key (credit_key),
  KEY idx_credit_leases_board (board_key, state, enqueued_at),
  KEY idx_credit_leases_state (state, enqueued_at),
  KEY idx_credit_leases_board_gen (board_key, bind_world_gen),
  KEY idx_credit_leases_revoked (revoked_at)
);

-- ──────────────────────────────────────────────────────────────────────────────
-- 事故证明仓（只追加）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fund_credit_fence_cases (
  id                  VARCHAR(64)   NOT NULL,
  kind                VARCHAR(32)   NOT NULL,
  credit_key          VARCHAR(191)  NOT NULL,
  lease_id            VARCHAR(64)   NOT NULL DEFAULT '',
  board_key           VARCHAR(191)  NOT NULL,
  bind_world_gen      BIGINT        NOT NULL DEFAULT 0,
  current_world_gen   BIGINT        NOT NULL DEFAULT 0,
  epoch               BIGINT        NOT NULL DEFAULT 0,
  token               VARCHAR(64)   NOT NULL DEFAULT '',
  conflict_lease_id   VARCHAR(64)   NOT NULL DEFAULT '',
  evidence            VARCHAR(4096) NOT NULL DEFAULT '',
  detected_at         DATETIME(3)   NOT NULL,
  exported_at         DATETIME(3)            DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_credit_cases_key (credit_key, detected_at),
  KEY idx_credit_cases_kind (kind, detected_at),
  KEY idx_credit_cases_board (board_key, detected_at),
  KEY idx_credit_cases_export (exported_at, detected_at),
  KEY idx_credit_cases_lease (lease_id, detected_at)
);

-- ──────────────────────────────────────────────────────────────────────────────
-- 投递证明（只追加；Deliver 成功后写一行）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fund_credit_proofs (
  id              VARCHAR(64)   NOT NULL,
  credit_key      VARCHAR(191)  NOT NULL,
  lease_id        VARCHAR(64)   NOT NULL,
  board_key       VARCHAR(191)  NOT NULL,
  world_gen       BIGINT        NOT NULL,
  lane_gen        BIGINT        NOT NULL,
  epoch           BIGINT        NOT NULL,
  amount          VARCHAR(64)   NOT NULL DEFAULT '',
  proved_at       DATETIME(3)   NOT NULL,
  exported_at     DATETIME(3)            DEFAULT NULL,
  created_at      DATETIME(3)   NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_credit_proofs_key (credit_key),
  KEY idx_credit_proofs_board (board_key, proved_at),
  KEY idx_credit_proofs_export (exported_at, proved_at),
  KEY idx_credit_proofs_lease (lease_id, proved_at)
);
