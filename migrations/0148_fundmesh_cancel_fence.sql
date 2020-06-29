-- 功能：资金 mesh 取消记录、漂移证明与静默兜底拒绝证明。
-- 重点：withdraw_id 唯一；证明只增。
-- 风险点：record_id 禁止缩短；时间 DATETIME(3) UTC。
-- Author: Blaise
-- 作者：TianpengLan
-- Date: 2020-12-04

CREATE TABLE IF NOT EXISTS fundmesh_cancel_records (
  record_id        VARCHAR(64)   NOT NULL,
  withdraw_id      VARCHAR(64)   NOT NULL,
  user_id          VARCHAR(64)   NOT NULL DEFAULT '',
  actor            VARCHAR(64)   NOT NULL DEFAULT '',
  reason           VARCHAR(255)  NOT NULL DEFAULT '',
  idempotency_key  VARCHAR(128)  NOT NULL DEFAULT '',
  entry            VARCHAR(16)   NOT NULL DEFAULT '',
  state            VARCHAR(16)   NOT NULL DEFAULT '',
  phase            VARCHAR(16)   NOT NULL DEFAULT '',
  code             VARCHAR(32)   NOT NULL DEFAULT '',
  remote_ok        TINYINT       NOT NULL DEFAULT 0,
  note             VARCHAR(255)  NOT NULL DEFAULT '',
  created_at       DATETIME(3)   NOT NULL,
  updated_at       DATETIME(3)   NOT NULL,
  PRIMARY KEY (record_id),
  UNIQUE KEY uk_fm_cancel_wd (withdraw_id),
  KEY idx_fm_cancel_idem (idempotency_key),
  KEY idx_fm_cancel_code (code, updated_at)
);

CREATE TABLE IF NOT EXISTS fundmesh_drift_proofs (
  proof_id     VARCHAR(64)   NOT NULL,
  kind         VARCHAR(32)   NOT NULL,
  missing_csv  VARCHAR(1024) NOT NULL DEFAULT '',
  mismatch_csv VARCHAR(1024) NOT NULL DEFAULT '',
  closed       TINYINT       NOT NULL DEFAULT 1,
  detail       VARCHAR(255)  NOT NULL DEFAULT '',
  detected_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (proof_id),
  KEY idx_fm_drift_kind (kind, detected_at)
);

CREATE TABLE IF NOT EXISTS fundmesh_fallback_proofs (
  proof_id     VARCHAR(64)   NOT NULL,
  withdraw_id  VARCHAR(64)   NOT NULL,
  remote_err   VARCHAR(255)  NOT NULL DEFAULT '',
  local_tried  TINYINT       NOT NULL DEFAULT 0,
  denied       TINYINT       NOT NULL DEFAULT 1,
  detected_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (proof_id),
  KEY idx_fm_fb_wd (withdraw_id, detected_at)
);
