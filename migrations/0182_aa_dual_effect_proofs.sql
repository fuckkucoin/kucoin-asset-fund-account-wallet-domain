-- 双控资金放行效果证明表。
-- proof_hash = SHA-256(request_id ‖ ticket_id ‖ fund_action_id ‖ gen ‖ actor)。
-- 证明缺失时不得将 ReleaseCase 标记为 executed。
-- 两个唯一约束防止同 request/gen 组合双录。
--
-- 迁移编号：0182
-- 作者：King
-- 日期：2022.03.12

CREATE TABLE IF NOT EXISTS dual_effect_proofs (
    id             VARCHAR(36)  NOT NULL PRIMARY KEY,
    request_id     VARCHAR(36)  NOT NULL,
    ticket_id      VARCHAR(36)  NOT NULL,
    gen            BIGINT       NOT NULL DEFAULT 0,
    fund_action_id VARCHAR(128) NOT NULL DEFAULT '',
    actor          VARCHAR(128) NOT NULL DEFAULT '',
    proof_hash     VARCHAR(64)  NOT NULL,
    signed_at      DATETIME(3)  NOT NULL,
    fence_epoch    BIGINT       NOT NULL DEFAULT 0,
    fence_token    VARCHAR(64)  NOT NULL DEFAULT '',
    created_at     DATETIME(3)  NOT NULL,
    UNIQUE KEY uk_request_gen (request_id, gen),
    UNIQUE KEY uk_proof_hash (proof_hash),
    KEY idx_ticket_id (ticket_id),
    KEY idx_signed_at (signed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='资金放行效果证明仓；proof_hash 防重放，缺失则禁 executed';

-- 双控放行流转案例主表（ReleaseCase）。
-- 串联 dualctl Request → 放行世代 → 票证 → 效果证明；状态机轴线。
CREATE TABLE IF NOT EXISTS dual_release_cases (
    id             VARCHAR(36)  NOT NULL PRIMARY KEY,
    request_id     VARCHAR(36)  NOT NULL,
    ticket_id      VARCHAR(36)  NOT NULL DEFAULT '',
    gen            BIGINT       NOT NULL DEFAULT 0,
    state          VARCHAR(32)  NOT NULL DEFAULT 'ready',
    actor          VARCHAR(128) NOT NULL DEFAULT '',
    fund_action_id VARCHAR(128) NOT NULL DEFAULT '',
    effect_proof_id VARCHAR(36) NOT NULL DEFAULT '',
    abort_reason   VARCHAR(256) NOT NULL DEFAULT '',
    fence_epoch    BIGINT       NOT NULL DEFAULT 0,
    fence_token    VARCHAR(64)  NOT NULL DEFAULT '',
    created_at     DATETIME(3)  NOT NULL,
    updated_at     DATETIME(3)  NOT NULL,
    UNIQUE KEY uk_request_id (request_id),
    KEY idx_state (state),
    KEY idx_ticket_id (ticket_id),
    KEY idx_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='双控放行案例主表；FSM ready→unlocking→executing→executed/expired/aborted';
