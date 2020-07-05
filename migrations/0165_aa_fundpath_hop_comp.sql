-- 资金路径同片 hop 绑定失败补偿闸：补偿仓、世代租约、双移证明。
--
-- 表设计原则：
--   1. fundpath_hop_comps：每次 BindTransfer 失败建一行（同 hop_idem 幂等）。
--   2. fundpath_comp_leases：每次申请补偿世代建一行（到期自动 expired）。
--   3. fundpath_double_move_proofs：双移/漏补只追加，不修改。
--
-- 作者：WesVoss
-- 日期：2024.06.02

-- ──────────────────────────────────────────────────────────────────────────────
-- 补偿仓
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fundpath_hop_comps (
  id              VARCHAR(64)    NOT NULL,
  hop_idem        VARCHAR(191)   NOT NULL,
  user_id         VARCHAR(64)    NOT NULL,
  asset           VARCHAR(16)    NOT NULL,
  amount          DECIMAL(36,18) NOT NULL,
  from_book       VARCHAR(32)    NOT NULL,
  to_book         VARCHAR(32)    NOT NULL,
  move_id         VARCHAR(128)   NOT NULL,
  bind_err        VARCHAR(512)   NOT NULL DEFAULT '',
  state           VARCHAR(32)    NOT NULL DEFAULT 'open',
  generation      BIGINT         NOT NULL DEFAULT 1,
  same_shard      TINYINT(1)     NOT NULL DEFAULT 1,
  actor           VARCHAR(64)    NOT NULL DEFAULT '',
  region          VARCHAR(32)    NOT NULL DEFAULT '',
  retry_count     INT            NOT NULL DEFAULT 0,
  forced_by       VARCHAR(128)   NOT NULL DEFAULT '',
  force_reason    VARCHAR(256)   NOT NULL DEFAULT '',
  created_at      DATETIME(3)    NOT NULL,
  updated_at      DATETIME(3)    NOT NULL,
  closed_at       DATETIME(3)             DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_hop_comps_idem (hop_idem),
  KEY idx_hop_comps_user_state (user_id, state, created_at),
  KEY idx_hop_comps_state (state, created_at)
);

-- ──────────────────────────────────────────────────────────────────────────────
-- 补偿世代租约
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fundpath_comp_leases (
  id          VARCHAR(64)  NOT NULL,
  case_id     VARCHAR(64)  NOT NULL,
  generation  BIGINT       NOT NULL,
  token       VARCHAR(64)  NOT NULL,
  state       VARCHAR(16)  NOT NULL DEFAULT 'open',
  expires_at  DATETIME(3)  NOT NULL,
  bound_at    DATETIME(3)           DEFAULT NULL,
  done_at     DATETIME(3)           DEFAULT NULL,
  expired_at  DATETIME(3)           DEFAULT NULL,
  created_at  DATETIME(3)  NOT NULL,
  PRIMARY KEY (id),
  KEY idx_comp_leases_case (case_id, generation),
  KEY idx_comp_leases_expires (expires_at, state)
);

-- ──────────────────────────────────────────────────────────────────────────────
-- 双移证明（只追加）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fundpath_double_move_proofs (
  id                  VARCHAR(64)    NOT NULL,
  case_id             VARCHAR(64)    NOT NULL,
  kind                VARCHAR(32)    NOT NULL,
  hop_idem            VARCHAR(191)   NOT NULL,
  user_id             VARCHAR(64)    NOT NULL,
  asset               VARCHAR(16)    NOT NULL,
  amount              DECIMAL(36,18) NOT NULL,
  move_id             VARCHAR(128)   NOT NULL,
  compensate_move_id  VARCHAR(128)            DEFAULT NULL,
  second_move_id      VARCHAR(128)            DEFAULT NULL,
  evidence            VARCHAR(2048)  NOT NULL DEFAULT '',
  detected_at         DATETIME(3)    NOT NULL,
  exported_at         DATETIME(3)             DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_double_move_case (case_id),
  KEY idx_double_move_user (user_id, detected_at),
  KEY idx_double_move_kind (kind, detected_at)
);
