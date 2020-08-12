-- 功能：资金 mesh 出站幂等仓、扣减流水与重复提交证明。
-- 重点：idempotency_key 唯一；成功扣减行不可覆盖。
-- 风险点：record_id 禁止缩短；时间 DATETIME(3) UTC。
-- Author: Hux
-- 作者：Axel
-- Date: 2020-12-14

CREATE TABLE IF NOT EXISTS fundmesh_outbound_records (
  record_id        VARCHAR(64)    NOT NULL,
  withdraw_id      VARCHAR(64)    NOT NULL DEFAULT '',
  user_id          VARCHAR(64)    NOT NULL DEFAULT '',
  actor            VARCHAR(64)    NOT NULL DEFAULT '',
  asset            VARCHAR(32)    NOT NULL DEFAULT '',
  amount           VARCHAR(64)    NOT NULL DEFAULT '',
  chain            VARCHAR(32)    NOT NULL DEFAULT '',
  dest             VARCHAR(256)   NOT NULL DEFAULT '',
  memo             VARCHAR(128)   NOT NULL DEFAULT '',
  tag              VARCHAR(64)    NOT NULL DEFAULT '',
  idempotency_key  VARCHAR(128)   NOT NULL,
  scheme           VARCHAR(16)    NOT NULL DEFAULT '',
  kind             VARCHAR(8)     NOT NULL DEFAULT '',
  fingerprint      CHAR(64)       NOT NULL DEFAULT '',
  canon            VARCHAR(512)   NOT NULL DEFAULT '',
  state            VARCHAR(16)    NOT NULL DEFAULT '',
  phase            VARCHAR(16)    NOT NULL DEFAULT '',
  code             VARCHAR(32)    NOT NULL DEFAULT '',
  remote_ok        TINYINT        NOT NULL DEFAULT 0,
  debit_count      INT            NOT NULL DEFAULT 0,
  note             VARCHAR(255)   NOT NULL DEFAULT '',
  created_at       DATETIME(3)    NOT NULL,
  updated_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (record_id),
  UNIQUE KEY uk_fm_out_idem (idempotency_key),
  KEY idx_fm_out_wd (withdraw_id),
  KEY idx_fm_out_user (user_id, updated_at),
  KEY idx_fm_out_code (code, updated_at)
);

CREATE TABLE IF NOT EXISTS fundmesh_debit_journal (
  line_id          VARCHAR(64)    NOT NULL,
  withdraw_id      VARCHAR(64)    NOT NULL DEFAULT '',
  idempotency_key  VARCHAR(128)   NOT NULL,
  fingerprint      CHAR(64)       NOT NULL DEFAULT '',
  amount           VARCHAR(64)    NOT NULL DEFAULT '',
  asset            VARCHAR(32)    NOT NULL DEFAULT '',
  user_id          VARCHAR(64)    NOT NULL DEFAULT '',
  ok               TINYINT        NOT NULL DEFAULT 0,
  remote_ok        TINYINT        NOT NULL DEFAULT 0,
  note             VARCHAR(255)   NOT NULL DEFAULT '',
  created_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (line_id),
  UNIQUE KEY uk_fm_debit_idem (idempotency_key),
  KEY idx_fm_debit_wd (withdraw_id, created_at)
);

CREATE TABLE IF NOT EXISTS fundmesh_outbound_proofs (
  proof_id         VARCHAR(80)    NOT NULL,
  kind             VARCHAR(32)    NOT NULL,
  withdraw_id      VARCHAR(64)    NOT NULL DEFAULT '',
  idempotency_key  VARCHAR(128)   NOT NULL DEFAULT '',
  fingerprint      CHAR(64)       NOT NULL DEFAULT '',
  prior_phase      VARCHAR(16)    NOT NULL DEFAULT '',
  prior_code       VARCHAR(32)    NOT NULL DEFAULT '',
  debit_attempt    INT            NOT NULL DEFAULT 0,
  blocked          TINYINT        NOT NULL DEFAULT 1,
  detail           VARCHAR(255)   NOT NULL DEFAULT '',
  detected_at      DATETIME(3)    NOT NULL,
  PRIMARY KEY (proof_id),
  KEY idx_fm_op_kind (kind, detected_at),
  KEY idx_fm_op_idem (idempotency_key)
);
