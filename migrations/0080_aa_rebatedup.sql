-- 返佣批次双结与重放封堵引擎表族。
-- 含：rebate_settle_batches、rebate_batch_credits、rebate_dup_cases、settle_leases、dup_proofs。
-- 不含错贷记 / 争议 / 红包相关表（与 r6 B / r7 G / r8 E 正交）。
--
-- 作者：Eco
-- 日期：2025.07.11

-- ---- rebate_settle_batches ----
-- 结算批次世代记录。状态机：open→settling→settled / open→sealed / settling→sealed。
-- settle_batch_id 全局唯一；(period, asset, generation) 三元组唯一。
-- generation 在同 period/asset 下严格单调递增，不可跳跃或回退。
CREATE TABLE IF NOT EXISTS rebate_settle_batches (
    settle_batch_id  VARCHAR(36)     NOT NULL,
    period           VARCHAR(10)     NOT NULL,   -- 结算周期 YYYY-MM-DD
    asset            VARCHAR(16)     NOT NULL,   -- 资产代码
    generation       BIGINT          NOT NULL,   -- 单调递增世代号
    state            VARCHAR(16)     NOT NULL DEFAULT 'open',
    credit_count     INT             NOT NULL DEFAULT 0,
    posted_sum       DECIMAL(36,18)  NOT NULL DEFAULT 0,
    opened_at        DATETIME(3)     NOT NULL,
    settling_at      DATETIME(3)     NULL,
    settled_at       DATETIME(3)     NULL,
    sealed_at        DATETIME(3)     NULL,
    seal_reason      VARCHAR(512)    NOT NULL DEFAULT '',
    sealed_by        VARCHAR(256)    NOT NULL DEFAULT '',
    created_at       DATETIME(3)     NOT NULL,
    updated_at       DATETIME(3)     NOT NULL,
    PRIMARY KEY (settle_batch_id),
    UNIQUE KEY uq_rsb_period_asset_gen (period, asset, generation),
    KEY idx_rsb_period_asset_state (period, asset, state),
    KEY idx_rsb_state_created (state, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='返佣结算批次世代记录';

-- ---- rebate_batch_credits ----
-- Credit 挂入批次关联表（N 对 1，一个 Credit 只属于一个批次）。
CREATE TABLE IF NOT EXISTS rebate_batch_credits (
    batch_id         VARCHAR(36)     NOT NULL,
    credit_id        VARCHAR(36)     NOT NULL,
    attached_at      DATETIME(3)     NOT NULL,
    PRIMARY KEY (batch_id, credit_id),
    KEY idx_rbc_credit (credit_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Credit 挂入批次关联';

-- ---- rebate_dup_cases ----
-- 双结案件记录。状态机：detected→reversed / detected→ignored / detected→escalated→reversed。
-- 不复用 r6 错贷记 Case 表；键空间独立（rebate settle/credit 专属）。
CREATE TABLE IF NOT EXISTS rebate_dup_cases (
    dup_case_id      VARCHAR(36)     NOT NULL,
    user_id          VARCHAR(64)     NOT NULL,
    asset            VARCHAR(16)     NOT NULL,
    period           VARCHAR(10)     NOT NULL,
    batch_id_1       VARCHAR(36)     NOT NULL,   -- 先到批次（较小世代）
    batch_gen_1      BIGINT          NOT NULL,
    batch_id_2       VARCHAR(36)     NOT NULL,   -- 重复批次（较大世代）
    batch_gen_2      BIGINT          NOT NULL,
    dup_amount       DECIMAL(36,18)  NOT NULL DEFAULT 0,
    state            VARCHAR(16)     NOT NULL DEFAULT 'detected',
    detected_at      DATETIME(3)     NOT NULL,
    escalated_at     DATETIME(3)     NULL,
    reversed_at      DATETIME(3)     NULL,
    ignored_at       DATETIME(3)     NULL,
    ignore_reason    VARCHAR(512)    NOT NULL DEFAULT '',
    reversed_by      VARCHAR(128)    NOT NULL DEFAULT '',
    reverse_proof_id VARCHAR(36)     NOT NULL DEFAULT '',
    created_at       DATETIME(3)     NOT NULL,
    updated_at       DATETIME(3)     NOT NULL,
    PRIMARY KEY (dup_case_id),
    UNIQUE KEY uq_rdc_batch_pair (batch_id_1, batch_id_2),
    KEY idx_rdc_state_detected (state, detected_at),
    KEY idx_rdc_user_period (user_id, period, asset),
    KEY idx_rdc_asset_period (asset, period, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='返佣双结案件记录';

-- ---- settle_leases ----
-- Credit↔Settle 互斥租约记录。同一 (batch_id, kind) 只存在一个活跃租约。
-- TTL 强制：expires_at 超时后视为自动释放。
CREATE TABLE IF NOT EXISTS settle_leases (
    lease_id         VARCHAR(36)     NOT NULL,
    batch_id         VARCHAR(36)     NOT NULL,
    user_id          VARCHAR(64)     NOT NULL DEFAULT '',
    kind             VARCHAR(8)      NOT NULL,  -- 'credit' | 'settle'
    holder_key       VARCHAR(256)    NOT NULL,
    acquired_at      DATETIME(3)     NOT NULL,
    expires_at       DATETIME(3)     NOT NULL,
    released_at      DATETIME(3)     NULL,
    released         TINYINT(1)      NOT NULL DEFAULT 0,
    updated_at       DATETIME(3)     NOT NULL,
    PRIMARY KEY (lease_id),
    KEY idx_sl_batch_kind_active (batch_id, kind, released, expires_at),
    KEY idx_sl_expires (expires_at, released)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Credit-Settle互斥租约';

-- ---- dup_proofs ----
-- 双结冲正证明记录（append-only）。
-- proof_id 全局唯一；写入后不可修改（insert-only 语义，ON DUPLICATE KEY 仅更新 reverse_led_id）。
-- checksum = SHA-256(proof_id + "|" + dup_case_id + "|" + reverse_amt)。
CREATE TABLE IF NOT EXISTS dup_proofs (
    dup_proof_id     VARCHAR(36)     NOT NULL,
    dup_case_id      VARCHAR(36)     NOT NULL,
    batch_id         VARCHAR(36)     NOT NULL,   -- 被冲正批次
    user_id          VARCHAR(64)     NOT NULL,
    asset            VARCHAR(16)     NOT NULL,
    reverse_amt      DECIMAL(36,18)  NOT NULL DEFAULT 0,
    reverse_led_id   VARCHAR(36)     NOT NULL DEFAULT '',  -- 账本冲正流水号（可回填）
    checksum         VARCHAR(64)     NOT NULL,
    issued_by        VARCHAR(128)    NOT NULL,
    issued_at        DATETIME(3)     NOT NULL,
    PRIMARY KEY (dup_proof_id),
    UNIQUE KEY uq_dp_case (dup_case_id),    -- 一个案件一个证明
    KEY idx_dp_batch (batch_id),
    KEY idx_dp_issued (issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='双结冲正证明（append-only）';
