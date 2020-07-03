-- 功能：出箱泵投递效果世代、半截 Mark 案与双效果证明表。
-- 重点：
--   outbox_deliver_gens 绑定 outbox_id、payload_hash、TTL 与世代。
--   outbox_dup_proofs 记录第二次全额效果。
--   outbox_half_mark_cases 覆盖投递成功未 Mark。
-- 风险点：gen_id/proof_id/case_id 禁止缩短；时间 DATETIME(3) UTC。
-- Author: TianpengLan
-- 作者：Kinsley
-- Date: 2020-12-02

CREATE TABLE IF NOT EXISTS outbox_deliver_gens (
  gen_id            VARCHAR(64)   NOT NULL,
  outbox_id         VARCHAR(64)   NOT NULL,
  business_key      VARCHAR(128)  NOT NULL DEFAULT '',
  kind              VARCHAR(64)   NOT NULL DEFAULT '',
  payload_hash      VARCHAR(64)   NOT NULL,
  effect_key        VARCHAR(64)   NOT NULL,
  generation        BIGINT        NOT NULL DEFAULT 0,
  state             VARCHAR(16)   NOT NULL DEFAULT 'issued',
  ttl_nanos         BIGINT        NOT NULL DEFAULT 0,
  deadline          DATETIME(3)   NOT NULL,
  issued_at         DATETIME(3)   NOT NULL,
  delivered_at      DATETIME(3)   NULL,
  marked_at         DATETIME(3)   NULL,
  expired_at        DATETIME(3)   NULL,
  applied           TINYINT       NOT NULL DEFAULT 0,
  handler_invoked   TINYINT       NOT NULL DEFAULT 0,
  actor             VARCHAR(64)   NOT NULL DEFAULT '',
  note              VARCHAR(255)  NOT NULL DEFAULT '',
  idempotency_key   VARCHAR(128)  NOT NULL DEFAULT '',
  PRIMARY KEY (gen_id),
  KEY idx_odg_outbox (outbox_id, generation),
  KEY idx_odg_effect (effect_key),
  KEY idx_odg_state (state, deadline)
);

CREATE TABLE IF NOT EXISTS outbox_dup_proofs (
  proof_id      VARCHAR(64)   NOT NULL,
  gen_id        VARCHAR(64)   NOT NULL DEFAULT '',
  outbox_id     VARCHAR(64)   NOT NULL,
  effect_key    VARCHAR(64)   NOT NULL DEFAULT '',
  kind          VARCHAR(32)   NOT NULL,
  generation    BIGINT        NOT NULL DEFAULT 0,
  payload_hash  VARCHAR(64)   NOT NULL DEFAULT '',
  actor         VARCHAR(64)   NOT NULL DEFAULT '',
  detail        VARCHAR(255)  NOT NULL DEFAULT '',
  detected_at   DATETIME(3)   NOT NULL,
  exported      TINYINT       NOT NULL DEFAULT 0,
  exported_at   DATETIME(3)   NULL,
  note          VARCHAR(255)  NOT NULL DEFAULT '',
  PRIMARY KEY (proof_id),
  KEY idx_odp_outbox (outbox_id, detected_at),
  KEY idx_odp_export (exported, detected_at),
  KEY idx_odp_kind (kind)
);

CREATE TABLE IF NOT EXISTS outbox_half_mark_cases (
  case_id        VARCHAR(64)   NOT NULL,
  gen_id         VARCHAR(64)   NOT NULL,
  outbox_id      VARCHAR(64)   NOT NULL,
  effect_key     VARCHAR(64)   NOT NULL DEFAULT '',
  state          VARCHAR(16)   NOT NULL DEFAULT 'open',
  reason         VARCHAR(64)   NOT NULL DEFAULT '',
  opened_at      DATETIME(3)   NOT NULL,
  resumed_at     DATETIME(3)   NULL,
  abandoned_at   DATETIME(3)   NULL,
  resume_token   VARCHAR(64)   NOT NULL DEFAULT '',
  resume_holder  VARCHAR(64)   NOT NULL DEFAULT '',
  resume_until   DATETIME(3)   NULL,
  resume_count   INT           NOT NULL DEFAULT 0,
  peer_acked     TINYINT       NOT NULL DEFAULT 0,
  note           VARCHAR(255)  NOT NULL DEFAULT '',
  PRIMARY KEY (case_id),
  KEY idx_ohmc_outbox (outbox_id, state),
  KEY idx_ohmc_gen (gen_id)
);

CREATE TABLE IF NOT EXISTS outbox_applied_effects (
  effect_key   VARCHAR(64)  NOT NULL,
  gen_id       VARCHAR(64)  NOT NULL,
  outbox_id    VARCHAR(64)  NOT NULL,
  generation   BIGINT       NOT NULL DEFAULT 0,
  applied_at   DATETIME(3)  NOT NULL,
  PRIMARY KEY (effect_key),
  KEY idx_oae_outbox (outbox_id)
);

CREATE TABLE IF NOT EXISTS outbox_pump_fence_audit (
  audit_id     VARCHAR(64)   NOT NULL,
  gen_id       VARCHAR(64)   NOT NULL DEFAULT '',
  outbox_id    VARCHAR(64)   NOT NULL DEFAULT '',
  actor        VARCHAR(64)   NOT NULL DEFAULT '',
  action       VARCHAR(32)   NOT NULL,
  outcome      VARCHAR(48)   NOT NULL DEFAULT '',
  detail       VARCHAR(255)  NOT NULL DEFAULT '',
  generation   BIGINT        NOT NULL DEFAULT 0,
  at           DATETIME(3)   NOT NULL,
  PRIMARY KEY (audit_id),
  KEY idx_opfa_outbox (outbox_id, at),
  KEY idx_opfa_gen (gen_id, at)
);
