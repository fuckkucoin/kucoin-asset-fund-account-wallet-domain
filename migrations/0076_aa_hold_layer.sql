-- 功能：分层冻结分类账——HoldToken、释放证明、缺口单、释放审计。
-- 重点：
--   hold_tokens.state 枚举：active / releasing / released / broken。
-- 作者：Axel
--   hold_gaps.state 枚举：open / acked / closed。
--   release_proofs.proof_ref 全局唯一（幂等键）；写入后只读。
--   hold_release_audits 追加写，不可删改，供合规审计。
-- 风险点：
--   token amount 用 VARCHAR(64) 十进制字符串，避免浮点精度损失。
--   proof_ref 建唯一索引；并发写时数据库层保证幂等。
--   同一 (token_id, gap_kind) 只允许一个 open/acked 缺口单——应用层保证，
--   数据库层无复合唯一索引（state 变化频繁），需应用层幂等。

CREATE TABLE IF NOT EXISTS hold_tokens (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id         VARCHAR(64)  NOT NULL,
  asset           VARCHAR(16)  NOT NULL,
  book            VARCHAR(32)  NOT NULL DEFAULT 'funding',
  amount          VARCHAR(64)  NOT NULL,
  layer           VARCHAR(16)  NOT NULL,               -- legal / aml / risk / ops
  reason_code     VARCHAR(64)  NOT NULL,
  ledger_ref      VARCHAR(128) NOT NULL DEFAULT '',    -- 账本冻结科目引用
  idem_key        VARCHAR(128) NOT NULL,               -- 幂等键，全局唯一
  approval_ref    VARCHAR(128) NOT NULL DEFAULT '',    -- 审批单 ID
  note            VARCHAR(512) NOT NULL DEFAULT '',
  state           VARCHAR(16)  NOT NULL DEFAULT 'active',
  created_at      DATETIME(3)  NOT NULL,
  updated_at      DATETIME(3)  NOT NULL,
  released_at     DATETIME(3)  NULL,
  broken_at       DATETIME(3)  NULL,
  UNIQUE KEY uk_hold_token_idem (idem_key),
  KEY idx_hold_token_user  (user_id, asset, state),
  KEY idx_hold_token_layer (layer, state),
  KEY idx_hold_token_ledger (ledger_ref)
);

CREATE TABLE IF NOT EXISTS hold_layers (
  -- 静态字典表：记录各层元数据，便于 ops dashboard 查询。
  -- 在应用启动时由 holdcat.AllLayers() 驱动 upsert，不由迁移写死数据。
  code        VARCHAR(16) NOT NULL PRIMARY KEY,
  name        VARCHAR(32) NOT NULL,
  priority    INT         NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS release_proofs (
  proof_id     VARCHAR(64)  NOT NULL PRIMARY KEY,
  proof_ref    VARCHAR(128) NOT NULL,               -- 等于 idem_key；全局唯一
  token_id     VARCHAR(64)  NOT NULL,
  user_id      VARCHAR(64)  NOT NULL,
  asset        VARCHAR(16)  NOT NULL,
  book         VARCHAR(32)  NOT NULL DEFAULT 'funding',
  amount       VARCHAR(64)  NOT NULL,
  frozen_layer VARCHAR(16)  NOT NULL,
  caller_layer VARCHAR(16)  NOT NULL,
  reason_code  VARCHAR(64)  NOT NULL,
  approval_ref VARCHAR(128) NOT NULL DEFAULT '',
  ledger_ref   VARCHAR(128) NOT NULL DEFAULT '',
  requester    VARCHAR(128) NOT NULL,
  note         VARCHAR(512) NOT NULL DEFAULT '',
  released_at  DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_release_proof_ref (proof_ref),
  KEY idx_release_proof_token  (token_id),
  KEY idx_release_proof_user   (user_id, asset),
  KEY idx_release_proof_layer  (frozen_layer, released_at)
);

CREATE TABLE IF NOT EXISTS hold_gaps (
  gap_id        VARCHAR(64)  NOT NULL PRIMARY KEY,
  token_id      VARCHAR(64)  NOT NULL DEFAULT '',    -- 无对应 token 时为 'holdcat:<row_id>'
  user_id       VARCHAR(64)  NOT NULL,
  asset         VARCHAR(16)  NOT NULL,
  book          VARCHAR(32)  NOT NULL DEFAULT 'funding',
  kind          VARCHAR(32)  NOT NULL,               -- orphan_tag / ledger_unfrozen / layer_violation / amount_drift / missing_token / timeout_escalation
  state         VARCHAR(16)  NOT NULL DEFAULT 'open',
  layer         VARCHAR(16)  NOT NULL DEFAULT '',
  token_amount  VARCHAR(64)  NOT NULL DEFAULT '',
  ledger_frozen VARCHAR(64)  NOT NULL DEFAULT '',
  detail        VARCHAR(1024) NOT NULL DEFAULT '',
  acked_by      VARCHAR(128) NOT NULL DEFAULT '',
  closed_by     VARCHAR(128) NOT NULL DEFAULT '',
  note          VARCHAR(512) NOT NULL DEFAULT '',
  opened_at     DATETIME(3)  NOT NULL,
  acked_at      DATETIME(3)  NULL,
  closed_at     DATETIME(3)  NULL,
  KEY idx_hold_gap_token  (token_id, kind),
  KEY idx_hold_gap_user   (user_id, asset, state),
  KEY idx_hold_gap_state  (state, opened_at),
  KEY idx_hold_gap_kind   (kind, state)
);

CREATE TABLE IF NOT EXISTS hold_release_audits (
  audit_id     VARCHAR(64)  NOT NULL PRIMARY KEY,
  token_id     VARCHAR(64)  NOT NULL,
  caller_layer VARCHAR(16)  NOT NULL,
  frozen_layer VARCHAR(16)  NOT NULL,
  reason_code  VARCHAR(64)  NOT NULL,
  requester    VARCHAR(128) NOT NULL,
  allowed      TINYINT(1)   NOT NULL DEFAULT 0,
  deny_reason  VARCHAR(255) NOT NULL DEFAULT '',
  proof_id     VARCHAR(64)  NOT NULL DEFAULT '',
  at           DATETIME(3)  NOT NULL,
  KEY idx_hold_audit_token   (token_id),
  KEY idx_hold_audit_at      (at),
  KEY idx_hold_audit_allowed (allowed, at)
);
