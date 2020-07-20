-- hop 执行令牌表（ap_hop_tokens）。
--
-- 设计约束：
--   - token_hash 唯一（防重放）。
--   - idem_key 唯一（幂等键，防重复签发）。
--   - policy_gen 在签发时锁定；hop 执行时必须与当前生效策略世代一致，否则挡写。
--   - 令牌一次性消费：consumed 后不可再用；expired 后不得绑定 hop。
--   - token_epoch 用于与 fi_debit_leases / dualctl ticket 键空间隔离，前缀 ht_。
--
-- 迁移编号：0188
-- 作者：Span / Likeo
-- 日期：2022.07.20

CREATE TABLE IF NOT EXISTS ap_hop_tokens (
    id           VARCHAR(36)   NOT NULL PRIMARY KEY          COMMENT 'UUID v4',
    user_id      VARCHAR(64)   NOT NULL DEFAULT ''           COMMENT 'hop 发起用户',
    asset        VARCHAR(16)   NOT NULL DEFAULT ''           COMMENT '币种',
    biz_kind     VARCHAR(32)   NOT NULL DEFAULT ''           COMMENT '业务类型（deposit/withdraw/transfer/...）',
    policy_gen   BIGINT        NOT NULL DEFAULT 0            COMMENT '签发时的策略世代号（hop 执行时复验）',
    policy_id    VARCHAR(36)   NOT NULL DEFAULT ''           COMMENT '关联 PolicyVersion.ID',
    state        VARCHAR(16)   NOT NULL DEFAULT 'issued'     COMMENT 'issued | consumed | expired | blocked',
    token_hash   VARCHAR(64)   NOT NULL DEFAULT ''           COMMENT 'SHA-256 hex（防重放）',
    idem_key     VARCHAR(128)  NOT NULL DEFAULT ''           COMMENT '幂等键（防重复签发）',
    issued_at    DATETIME(3)   NOT NULL                      COMMENT '签发时刻',
    expires_at   DATETIME(3)   NOT NULL                      COMMENT '过期时刻（IssuedAt + TTL）',
    consumed_at  DATETIME(3)                                 COMMENT '消费时刻（NULL=未消费）',
    hop_ref      VARCHAR(128)  NOT NULL DEFAULT ''           COMMENT '绑定的业务单据 ID（consumed 后填入）',
    block_reason VARCHAR(512)  NOT NULL DEFAULT ''           COMMENT '挡写原因（blocked 后填入）',
    token_epoch  BIGINT        NOT NULL DEFAULT 0            COMMENT '隔离纪元（毫秒，ht_ 前缀键空间）',
    created_at   DATETIME(3)   NOT NULL,
    updated_at   DATETIME(3)   NOT NULL,

    -- 防重放 / 防重复
    UNIQUE KEY uk_token_hash (token_hash),
    UNIQUE KEY uk_idem_key   (idem_key),

    KEY idx_user_biz        (user_id, biz_kind),
    KEY idx_state_expires   (state, expires_at),
    KEY idx_policy_gen      (policy_gen),
    KEY idx_issued_at       (issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='hop 执行令牌仓；policy_gen 防越权；token_hash 防重放；idem_key 防重复签发';
