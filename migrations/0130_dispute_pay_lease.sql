-- 功能：争议结案放款/扣回租约与证明表。
-- 重点：
--   dispute_pay_leases 绑定 resolve 放款；dispute_claw_leases 绑定 reject 扣回。
--   dispute_pay_proofs 只增；dispute_pay_audit 运维轨迹；dispute_case_generations 世代镜像。
-- 风险点：lease_id/proof_id 禁止缩短；时间 DATETIME(3) UTC。
-- Author: TianpengLan
-- 作者：TianpengLan
-- Date: 2020-12-08

CREATE TABLE IF NOT EXISTS dispute_pay_leases (
  lease_id          VARCHAR(64)   NOT NULL,
  case_id           VARCHAR(64)   NOT NULL,
  idempotency_key   VARCHAR(128)  NOT NULL DEFAULT '',
  user_id           VARCHAR(64)   NOT NULL,
  amount            VARCHAR(64)   NOT NULL,
  asset             VARCHAR(16)   NOT NULL,
  evidence_root     VARCHAR(64)   NOT NULL,
  decision          VARCHAR(16)   NOT NULL,
  pay_gen           BIGINT        NOT NULL DEFAULT 0,
  state             VARCHAR(24)   NOT NULL DEFAULT 'pay_issued',
  issued_by         VARCHAR(64)   NOT NULL DEFAULT '',
  issued_at         DATETIME(3)   NOT NULL,
  expire_at         DATETIME(3)   NOT NULL,
  bound_ledger_id   VARCHAR(64)   NOT NULL DEFAULT '',
  consumed_at       DATETIME(3)   NULL,
  revoked_at        DATETIME(3)   NULL,
  revoke_note       VARCHAR(255)  NOT NULL DEFAULT '',
  PRIMARY KEY (lease_id),
  UNIQUE KEY uk_dpl_case (case_id),
  KEY idx_dpl_idem (idempotency_key),
  KEY idx_dpl_state_expire (state, expire_at)
);

CREATE TABLE IF NOT EXISTS dispute_claw_leases (
  lease_id          VARCHAR(64)   NOT NULL,
  case_id           VARCHAR(64)   NOT NULL,
  idempotency_key   VARCHAR(128)  NOT NULL DEFAULT '',
  user_id           VARCHAR(64)   NOT NULL,
  amount            VARCHAR(64)   NOT NULL,
  asset             VARCHAR(16)   NOT NULL,
  evidence_root     VARCHAR(64)   NOT NULL,
  decision          VARCHAR(16)   NOT NULL,
  pay_gen           BIGINT        NOT NULL DEFAULT 0,
  state             VARCHAR(24)   NOT NULL DEFAULT 'claw_issued',
  release_ref       VARCHAR(128)  NOT NULL DEFAULT '',
  issued_by         VARCHAR(64)   NOT NULL DEFAULT '',
  issued_at         DATETIME(3)   NOT NULL,
  expire_at         DATETIME(3)   NOT NULL,
  bound_ledger_id   VARCHAR(64)   NOT NULL DEFAULT '',
  consumed_at       DATETIME(3)   NULL,
  revoked_at        DATETIME(3)   NULL,
  revoke_note       VARCHAR(255)  NOT NULL DEFAULT '',
  PRIMARY KEY (lease_id),
  UNIQUE KEY uk_dcl_case (case_id),
  KEY idx_dcl_state_expire (state, expire_at)
);

CREATE TABLE IF NOT EXISTS dispute_pay_proofs (
  proof_id    VARCHAR(64)   NOT NULL,
  case_id     VARCHAR(64)   NOT NULL,
  pay_gen     BIGINT        NOT NULL DEFAULT 0,
  ledger_id   VARCHAR(64)   NOT NULL DEFAULT '',
  lease_id    VARCHAR(64)   NOT NULL DEFAULT '',
  kind        VARCHAR(8)    NOT NULL DEFAULT 'pay',
  outcome     VARCHAR(32)   NOT NULL,
  detail      VARCHAR(255)  NOT NULL DEFAULT '',
  digest      VARCHAR(64)   NOT NULL DEFAULT '',
  created_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (proof_id),
  KEY idx_dpp_case (case_id, pay_gen),
  KEY idx_dpp_outcome (outcome, created_at)
);

CREATE TABLE IF NOT EXISTS dispute_pay_audit (
  audit_id  VARCHAR(64)   NOT NULL,
  case_id   VARCHAR(64)   NOT NULL,
  lease_id  VARCHAR(64)   NOT NULL DEFAULT '',
  actor     VARCHAR(64)   NOT NULL DEFAULT '',
  action    VARCHAR(32)   NOT NULL,
  detail    VARCHAR(255)  NOT NULL DEFAULT '',
  at        DATETIME(3)   NOT NULL,
  PRIMARY KEY (audit_id),
  KEY idx_dpa_case (case_id, at)
);

CREATE TABLE IF NOT EXISTS dispute_case_generations (
  case_id       VARCHAR(64)  NOT NULL,
  pay_gen       BIGINT       NOT NULL DEFAULT 1,
  evidence_root VARCHAR(64)  NOT NULL DEFAULT '',
  updated_at    DATETIME(3)  NOT NULL,
  PRIMARY KEY (case_id)
);
