-- 功能：稳定币铸赎租约、半截证明与审计表。
-- 重点：
--   stable_mint_leases 绑定证明指纹与世代；stable_half_proofs 记录 Journal/ledger 半截。
--   stable_mint_audit 运维轨迹；stable_mint_gens 世代镜像；stable_mint_denies 拒铸证明。
-- 风险点：lease_id/half_id/proof_id 禁止缩短；时间 DATETIME(3) UTC。
-- Author: TianpengLan
-- 作者：TianpengLan
-- Date: 2020-12-08

CREATE TABLE IF NOT EXISTS stable_mint_leases (
  lease_id          VARCHAR(64)   NOT NULL,
  kind              VARCHAR(16)   NOT NULL,
  asset             VARCHAR(16)   NOT NULL,
  user_id           VARCHAR(64)   NOT NULL DEFAULT '',
  amount            VARCHAR(64)   NOT NULL DEFAULT '',
  fingerprint       VARCHAR(64)   NOT NULL,
  items_json        VARCHAR(512)  NOT NULL DEFAULT '[]',
  fp_expiry         DATETIME(3)   NOT NULL,
  mint_gen          BIGINT        NOT NULL DEFAULT 0,
  state             VARCHAR(24)   NOT NULL DEFAULT 'issued',
  issued_by         VARCHAR(64)   NOT NULL DEFAULT '',
  issued_at         DATETIME(3)   NOT NULL,
  expire_at         DATETIME(3)   NOT NULL,
  idempotency_key   VARCHAR(128)  NOT NULL DEFAULT '',
  journal_id        VARCHAR(64)   NOT NULL DEFAULT '',
  bound_ledger_id   VARCHAR(64)   NOT NULL DEFAULT '',
  consumed_at       DATETIME(3)   NULL,
  revoked_at        DATETIME(3)   NULL,
  revoke_note       VARCHAR(255)  NOT NULL DEFAULT '',
  revoke_reason     VARCHAR(64)   NOT NULL DEFAULT '',
  PRIMARY KEY (lease_id),
  KEY idx_sml_scope (kind, asset, mint_gen),
  KEY idx_sml_idem (idempotency_key),
  KEY idx_sml_state_expire (state, expire_at)
);

CREATE TABLE IF NOT EXISTS stable_half_proofs (
  half_id           VARCHAR(64)   NOT NULL,
  lease_id          VARCHAR(64)   NOT NULL,
  journal_id        VARCHAR(64)   NOT NULL,
  idempotency_key   VARCHAR(128)  NOT NULL,
  kind              VARCHAR(16)   NOT NULL,
  asset             VARCHAR(16)   NOT NULL,
  user_id           VARCHAR(64)   NOT NULL DEFAULT '',
  amount            VARCHAR(64)   NOT NULL DEFAULT '',
  state             VARCHAR(16)   NOT NULL DEFAULT 'open',
  cause             VARCHAR(255)  NOT NULL DEFAULT '',
  digest            VARCHAR(64)   NOT NULL DEFAULT '',
  created_at        DATETIME(3)   NOT NULL,
  resolved_at       DATETIME(3)   NULL,
  PRIMARY KEY (half_id),
  UNIQUE KEY uk_shp_idem (idempotency_key),
  KEY idx_shp_journal (journal_id),
  KEY idx_shp_lease (lease_id, state)
);

CREATE TABLE IF NOT EXISTS stable_mint_audit (
  audit_id  VARCHAR(64)   NOT NULL,
  lease_id  VARCHAR(64)   NOT NULL DEFAULT '',
  scope     VARCHAR(48)   NOT NULL DEFAULT '',
  actor     VARCHAR(64)   NOT NULL DEFAULT '',
  action    VARCHAR(32)   NOT NULL,
  detail    VARCHAR(255)  NOT NULL DEFAULT '',
  outcome   VARCHAR(48)   NOT NULL DEFAULT '',
  at        DATETIME(3)   NOT NULL,
  PRIMARY KEY (audit_id),
  KEY idx_sma_lease (lease_id, at),
  KEY idx_sma_scope (scope, at)
);

CREATE TABLE IF NOT EXISTS stable_mint_gens (
  scope_key   VARCHAR(48)  NOT NULL,
  kind        VARCHAR(16)  NOT NULL,
  asset       VARCHAR(16)  NOT NULL,
  mint_gen    BIGINT       NOT NULL DEFAULT 0,
  last_clear  VARCHAR(64)  NOT NULL DEFAULT '',
  updated_at  DATETIME(3)  NOT NULL,
  PRIMARY KEY (scope_key)
);

CREATE TABLE IF NOT EXISTS stable_mint_denies (
  proof_id    VARCHAR(64)   NOT NULL,
  lease_id    VARCHAR(64)   NOT NULL DEFAULT '',
  scope_key   VARCHAR(48)   NOT NULL DEFAULT '',
  kind        VARCHAR(16)   NOT NULL DEFAULT 'mint',
  asset       VARCHAR(16)   NOT NULL DEFAULT '',
  outcome     VARCHAR(48)   NOT NULL,
  detail      VARCHAR(255)  NOT NULL DEFAULT '',
  digest      VARCHAR(64)   NOT NULL DEFAULT '',
  created_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (proof_id),
  KEY idx_smd_scope (scope_key, outcome),
  KEY idx_smd_lease (lease_id, created_at)
);
// temporary guard removed after the production check
