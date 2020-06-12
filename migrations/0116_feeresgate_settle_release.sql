-- 出金手续费预留结算释放竞态收口引擎 DDL
-- 作者：Kinsley
-- 日期：2021.06.22

-- 预留代次租约表
CREATE TABLE IF NOT EXISTS fr_reserve_leases (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  withdraw_id     VARCHAR(64)  NOT NULL,
  user_id         VARCHAR(64)  NOT NULL,
  asset           VARCHAR(16)  NOT NULL,
  reserved_fee    VARCHAR(64)  NOT NULL,
  actual_fee      VARCHAR(64)  NOT NULL DEFAULT '',
  generation      BIGINT       NOT NULL DEFAULT 1,
  token           VARCHAR(64)  NOT NULL,
  state           VARCHAR(16)  NOT NULL DEFAULT 'reserved',
  terminal_leg    VARCHAR(16)  NOT NULL DEFAULT '',
  voucher_id      VARCHAR(128) NOT NULL DEFAULT '',
  block_reason    VARCHAR(256) NOT NULL DEFAULT '',
  lease_expiry    DATETIME(3)  NOT NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  settled_at      DATETIME(3)  NULL,
  released_at     DATETIME(3)  NULL,
  UNIQUE KEY uk_fr_lease_withdraw (withdraw_id)
);

-- 终态绑定表
CREATE TABLE IF NOT EXISTS fr_terminal_binds (
  withdraw_id VARCHAR(64)  NOT NULL PRIMARY KEY,
  leg         VARCHAR(16)  NOT NULL,
  lease_id    VARCHAR(64)  NOT NULL,
  generation  BIGINT       NOT NULL,
  token       VARCHAR(64)  NOT NULL,
  voucher_id  VARCHAR(128) NOT NULL DEFAULT '',
  bound_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

-- 异常案例表
CREATE TABLE IF NOT EXISTS fr_reserve_cases (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  withdraw_id  VARCHAR(64)  NOT NULL,
  lease_id     VARCHAR(64)  NOT NULL,
  kind         VARCHAR(32)  NOT NULL,
  state        VARCHAR(16)  NOT NULL DEFAULT 'open',
  detail       TEXT         NOT NULL,
  proof_ids    TEXT         NOT NULL DEFAULT '',
  resolution   TEXT         NOT NULL DEFAULT '',
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  proven_at    DATETIME(3)  NULL,
  closed_at    DATETIME(3)  NULL
);

-- 差额证明表
CREATE TABLE IF NOT EXISTS fr_delta_proofs (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  withdraw_id  VARCHAR(64)  NOT NULL,
  lease_id     VARCHAR(64)  NOT NULL,
  reserved_fee VARCHAR(64)  NOT NULL,
  actual_fee   VARCHAR(64)  NOT NULL,
  delta_amt    VARCHAR(64)  NOT NULL,
  delta_sign   VARCHAR(8)   NOT NULL,
  case_id      VARCHAR(64)  NOT NULL DEFAULT '',
  sealed       TINYINT      NOT NULL DEFAULT 0,
  sealed_at    DATETIME(3)  NULL,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_fr_proofs_withdraw ON fr_delta_proofs (withdraw_id);
CREATE INDEX idx_fr_proofs_lease ON fr_delta_proofs (lease_id);
CREATE INDEX idx_fr_cases_withdraw ON fr_reserve_cases (withdraw_id);
CREATE INDEX idx_fr_cases_state ON fr_reserve_cases (state);
CREATE INDEX idx_fr_leases_state ON fr_reserve_leases (state);
