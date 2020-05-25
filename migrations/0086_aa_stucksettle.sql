-- 功能：滞留回收双结封堵引擎表结构。
-- 边界：
--   inc_stuck_generations — 每个 transfer_id 的回收效果世代号（单调递增）。
--   inc_recover_leases    — 单次 RecoverStuck 互斥租约（Acting 期间排他）。
--   inc_dup_cases         — 重复回收双结案件（Dup Recover Detector 产出）。
--   inc_effect_proofs     — 回收终态效果证明（append-only）。
-- 不变量：
--   inc_effect_proofs 无 UPDATE 权限；写入后只读。
--   inc_stuck_generations.current_gen 通过 UPDATE … SET current_gen = current_gen + 1 原子递增。
--   同一 transfer_id 同一 generation 最多存在一个 Acting 租约（业务层保证；DB 层索引辅助检测）。
--
-- 作者：Eco
-- 日期：2025.11.22

-- ── inc_stuck_generations ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS inc_stuck_generations (
    stuck_gen_id    VARCHAR(128)     NOT NULL,
    transfer_id     VARCHAR(128)     NOT NULL,
    current_gen     BIGINT           NOT NULL DEFAULT 0,
    last_acting_at  DATETIME(3)      NULL,
    terminated_at   DATETIME(3)      NULL,
    created_at      DATETIME(3)      NOT NULL,
    updated_at      DATETIME(3)      NOT NULL,
    PRIMARY KEY (stuck_gen_id),
    UNIQUE KEY uq_inc_gen_transfer (transfer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='transfer 级回收效果世代号';

-- ── inc_recover_leases ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS inc_recover_leases (
    lease_id        VARCHAR(128)     NOT NULL,
    transfer_id     VARCHAR(128)     NOT NULL,
    generation      BIGINT           NOT NULL,
    state           VARCHAR(32)      NOT NULL COMMENT 'open|acting|recovered|dup_blocked|expired',
    holder_key      VARCHAR(256)     NOT NULL COMMENT '持锁方幂等键',
    opened_at       DATETIME(3)      NOT NULL,
    acting_at       DATETIME(3)      NULL,
    recovered_at    DATETIME(3)      NULL,
    blocked_at      DATETIME(3)      NULL,
    expired_at      DATETIME(3)      NULL,
    expires_at      DATETIME(3)      NOT NULL COMMENT 'TTL 截止，超时视为 Expired',
    effect_proof_id VARCHAR(128)     NULL     COMMENT '关联效果证明 ID',
    block_reason    VARCHAR(256)     NULL,
    created_at      DATETIME(3)      NOT NULL,
    updated_at      DATETIME(3)      NOT NULL,
    PRIMARY KEY (lease_id),
    KEY idx_inc_lease_transfer_state (transfer_id, state),
    KEY idx_inc_lease_expires (expires_at),
    KEY idx_inc_lease_transfer_acting (transfer_id, acting_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='单次 RecoverStuck 互斥租约';

-- ── inc_dup_cases ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS inc_dup_cases (
    dup_case_id     VARCHAR(128)     NOT NULL,
    transfer_id     VARCHAR(128)     NOT NULL,
    generation      BIGINT           NOT NULL,
    first_lease_id  VARCHAR(128)     NOT NULL COMMENT '合法首次租约 ID',
    dup_lease_id    VARCHAR(128)     NOT NULL COMMENT '重复/被拦截租约 ID',
    state           VARCHAR(32)      NOT NULL COMMENT 'open|proven|closed',
    dup_proof_id    VARCHAR(128)     NULL     COMMENT '关联证明 ID',
    confirmed_by    VARCHAR(128)     NULL     COMMENT '确认人 1',
    co_confirmed_by VARCHAR(128)     NULL     COMMENT '确认人 2（双人鉴权）',
    detected_at     DATETIME(3)      NOT NULL,
    proven_at       DATETIME(3)      NULL,
    closed_at       DATETIME(3)      NULL,
    close_note      VARCHAR(512)     NULL,
    created_at      DATETIME(3)      NOT NULL,
    updated_at      DATETIME(3)      NOT NULL,
    PRIMARY KEY (dup_case_id),
    KEY idx_inc_dup_transfer (transfer_id),
    KEY idx_inc_dup_state (state),
    KEY idx_inc_dup_lease (dup_lease_id),
    UNIQUE KEY uq_inc_dup_lease_pair (first_lease_id, dup_lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='重复回收双结案件';

-- ── inc_effect_proofs ────────────────────────────────────────────────────────
-- append-only：环境应撤销 UPDATE/DELETE 权限。

CREATE TABLE IF NOT EXISTS inc_effect_proofs (
    effect_proof_id VARCHAR(128)     NOT NULL,
    transfer_id     VARCHAR(128)     NOT NULL,
    lease_id        VARCHAR(128)     NOT NULL,
    generation      BIGINT           NOT NULL,
    intent          VARCHAR(512)     NOT NULL COMMENT 'RecoverStuck:{transfer_id}:gen={n}',
    action          VARCHAR(64)      NOT NULL COMMENT 'wait|fee_bump|recover|fail|noop',
    ledger_ref      VARCHAR(256)     NULL     COMMENT '账本/广播只读引用',
    checksum        VARCHAR(128)     NOT NULL COMMENT 'SHA-256(intent|action|ledger_ref)',
    issued_by       VARCHAR(128)     NOT NULL,
    issued_at       DATETIME(3)      NOT NULL,
    PRIMARY KEY (effect_proof_id),
    UNIQUE KEY uq_inc_proof_lease (lease_id),
    KEY idx_inc_proof_transfer (transfer_id),
    KEY idx_inc_proof_issued (issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='回收终态效果证明（append-only）';
