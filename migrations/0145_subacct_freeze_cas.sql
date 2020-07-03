-- 功能：子账户冻结 CAS、冻后借记证明与速度窗吊销表。
-- 重点：
--   subacct_freeze_cas 绑定 parent_id、child_set_fingerprint、reason 与世代。
--   subacct_frozen_spend_proofs 记录冻后击穿并驱动补偿。
--   subacct_vel_revocations 吊销冻前速度窗 hit。
-- 风险点：freeze_cas_id/proof_id/revoke_id 禁止缩短；时间 DATETIME(3) UTC。
-- Author: TianpengLan
-- 作者：Kinsley
-- Date: 2020-12-02

CREATE TABLE IF NOT EXISTS subacct_freeze_cas (
  freeze_cas_id          VARCHAR(64)   NOT NULL,
  parent_id              VARCHAR(64)   NOT NULL,
  child_set_fingerprint  VARCHAR(64)   NOT NULL DEFAULT '',
  child_ids_json         VARCHAR(2048) NOT NULL DEFAULT '[]',
  reason                 VARCHAR(255)  NOT NULL DEFAULT '',
  generation             BIGINT        NOT NULL DEFAULT 0,
  state                  VARCHAR(16)   NOT NULL DEFAULT 'open',
  actor                  VARCHAR(64)   NOT NULL DEFAULT '',
  opened_at              DATETIME(3)   NOT NULL,
  frozen_at              DATETIME(3)   NULL,
  thawed_at              DATETIME(3)   NULL,
  superseded_by          VARCHAR(64)   NOT NULL DEFAULT '',
  idempotency_key        VARCHAR(128)  NOT NULL DEFAULT '',
  note                   VARCHAR(255)  NOT NULL DEFAULT '',
  PRIMARY KEY (freeze_cas_id),
  KEY idx_sfc_parent_gen (parent_id, generation),
  KEY idx_sfc_state_frozen (state, frozen_at),
  KEY idx_sfc_idem (parent_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS subacct_frozen_spend_proofs (
  proof_id                 VARCHAR(64)   NOT NULL,
  freeze_cas_id            VARCHAR(64)   NOT NULL,
  parent_id                VARCHAR(64)   NOT NULL DEFAULT '',
  child_id                 VARCHAR(64)   NOT NULL DEFAULT '',
  owner_id                 VARCHAR(64)   NOT NULL,
  asset                    VARCHAR(16)   NOT NULL,
  amount                   VARCHAR(64)   NOT NULL,
  debit_idem               VARCHAR(128)  NOT NULL DEFAULT '',
  detected_at              DATETIME(3)   NOT NULL,
  compensated              TINYINT       NOT NULL DEFAULT 0,
  compensate_credit_key    VARCHAR(160)  NOT NULL DEFAULT '',
  state                    VARCHAR(16)   NOT NULL DEFAULT 'open',
  acked_by                 VARCHAR(64)   NOT NULL DEFAULT '',
  acked_at                 DATETIME(3)   NULL,
  note                     VARCHAR(255)  NOT NULL DEFAULT '',
  rolled_back_available    VARCHAR(64)   NOT NULL DEFAULT '',
  PRIMARY KEY (proof_id),
  KEY idx_sfsp_cas (freeze_cas_id, state),
  KEY idx_sfsp_idem (debit_idem),
  KEY idx_sfsp_owner (owner_id, detected_at)
);

CREATE TABLE IF NOT EXISTS subacct_vel_revocations (
  revoke_id      VARCHAR(64)   NOT NULL,
  freeze_cas_id  VARCHAR(64)   NOT NULL,
  parent_id      VARCHAR(64)   NOT NULL DEFAULT '',
  owner_id       VARCHAR(64)   NOT NULL,
  generation     BIGINT        NOT NULL DEFAULT 0,
  frozen_at      DATETIME(3)   NOT NULL,
  window_from    DATETIME(3)   NOT NULL,
  window_to      DATETIME(3)   NOT NULL,
  hit_count      INT           NOT NULL DEFAULT 0,
  active         TINYINT       NOT NULL DEFAULT 1,
  note           VARCHAR(255)  NOT NULL DEFAULT '',
  PRIMARY KEY (revoke_id),
  KEY idx_svr_owner (owner_id, active),
  KEY idx_svr_cas (freeze_cas_id)
);

CREATE TABLE IF NOT EXISTS subacct_freeze_cas_audit (
  audit_id       VARCHAR(64)   NOT NULL,
  freeze_cas_id  VARCHAR(64)   NOT NULL DEFAULT '',
  parent_id      VARCHAR(64)   NOT NULL DEFAULT '',
  child_id       VARCHAR(64)   NOT NULL DEFAULT '',
  actor          VARCHAR(64)   NOT NULL DEFAULT '',
  action         VARCHAR(32)   NOT NULL,
  outcome        VARCHAR(48)   NOT NULL DEFAULT '',
  detail         VARCHAR(255)  NOT NULL DEFAULT '',
  generation     BIGINT        NOT NULL DEFAULT 0,
  at             DATETIME(3)   NOT NULL,
  PRIMARY KEY (audit_id),
  KEY idx_sfca_parent (parent_id, at),
  KEY idx_sfca_cas (freeze_cas_id, at)
);

CREATE TABLE IF NOT EXISTS subacct_freeze_cas_journals (
  journal_id        VARCHAR(64)   NOT NULL,
  freeze_cas_id     VARCHAR(64)   NOT NULL DEFAULT '',
  owner_id          VARCHAR(64)   NOT NULL,
  parent_id         VARCHAR(64)   NOT NULL DEFAULT '',
  child_id          VARCHAR(64)   NOT NULL DEFAULT '',
  asset             VARCHAR(16)   NOT NULL DEFAULT '',
  amount            VARCHAR(64)   NOT NULL DEFAULT '',
  idempotency_key   VARCHAR(128)  NOT NULL DEFAULT '',
  ok                TINYINT       NOT NULL DEFAULT 0,
  refused           VARCHAR(255)  NOT NULL DEFAULT '',
  at                DATETIME(3)   NOT NULL,
  after_frozen      TINYINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (journal_id),
  KEY idx_sfcj_owner (owner_id, at),
  KEY idx_sfcj_cas (freeze_cas_id, at)
);
