-- Migration 0050：liqplan 强平资金占用与部分扣押残留封堵
-- 表：liq_plans, seize_legs, mark_gates, liq_proofs
-- 作者：Lennox
-- 日期：2020.12.11

-- ─────────────────────────────────────────────
-- liq_plans：强平计划主表
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS liq_plans (
    id                  VARCHAR(64)   NOT NULL,
    user_id             VARCHAR(64)   NOT NULL,
    borrow_id           VARCHAR(64)   NOT NULL,
    collateral_id       VARCHAR(64)   NOT NULL,
    trigger_ltv_bps     INT           NOT NULL DEFAULT 0,
    cap_bps             INT           NOT NULL DEFAULT 0,
    collateral_asset    VARCHAR(32)   NOT NULL DEFAULT '',
    collateral_amount   VARCHAR(64)   NOT NULL DEFAULT '0',
    borrow_asset        VARCHAR(32)   NOT NULL DEFAULT '',
    borrow_amount       VARCHAR(64)   NOT NULL DEFAULT '0',
    occupy_token        VARCHAR(64)   NOT NULL DEFAULT '',
    mark_gate_id        VARCHAR(64)   NOT NULL DEFAULT '',
    mark_generation     BIGINT        NOT NULL DEFAULT 0,
    state               VARCHAR(32)   NOT NULL DEFAULT 'triggered',
    idempotency_key     VARCHAR(128)  NOT NULL DEFAULT '',
    proof_id            VARCHAR(64)   NOT NULL DEFAULT '',
    aborted_by          VARCHAR(256)  NOT NULL DEFAULT '',
    aborted_at          TIMESTAMP     NULL,
    abort_reason        VARCHAR(512)  NOT NULL DEFAULT '',
    fence_epoch         BIGINT        NOT NULL DEFAULT 0,
    fence_token         VARCHAR(64)   NOT NULL DEFAULT '',
    created_at          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_liq_plans_idem (idempotency_key),
    KEY idx_liq_plans_borrow (borrow_id),
    KEY idx_liq_plans_user (user_id),
    KEY idx_liq_plans_state (state),
    KEY idx_liq_plans_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='LTV强平计划主表';

-- ─────────────────────────────────────────────
-- seize_legs：扣押腿表
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS seize_legs (
    id                  VARCHAR(64)   NOT NULL,
    plan_id             VARCHAR(64)   NOT NULL,
    user_id             VARCHAR(64)   NOT NULL,
    asset               VARCHAR(32)   NOT NULL DEFAULT '',
    target_amount       VARCHAR(64)   NOT NULL DEFAULT '0',
    partial_amount      VARCHAR(64)   NOT NULL DEFAULT '0',
    hold_id             VARCHAR(64)   NOT NULL DEFAULT '',
    borrow_id           VARCHAR(64)   NOT NULL DEFAULT '',
    state               VARCHAR(32)   NOT NULL DEFAULT 'pending',
    fail_reason         VARCHAR(512)  NOT NULL DEFAULT '',
    fail_at             TIMESTAMP     NULL,
    retry_count         INT           NOT NULL DEFAULT 0,
    max_retry           INT           NOT NULL DEFAULT 3,
    idempotency_key     VARCHAR(128)  NOT NULL DEFAULT '',
    rolled_at           TIMESTAMP     NULL,
    suspended_at        TIMESTAMP     NULL,
    suspend_note        VARCHAR(512)  NOT NULL DEFAULT '',
    fence_epoch         BIGINT        NOT NULL DEFAULT 0,
    fence_token         VARCHAR(64)   NOT NULL DEFAULT '',
    created_at          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_seize_legs_idem (idempotency_key),
    KEY idx_seize_legs_plan (plan_id),
    KEY idx_seize_legs_state (state),
    KEY idx_seize_legs_borrow (borrow_id),
    KEY idx_seize_legs_created (created_at),
    CONSTRAINT fk_seize_legs_plan FOREIGN KEY (plan_id) REFERENCES liq_plans (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='扣押腿表（含部分失败残留）';

-- ─────────────────────────────────────────────
-- mark_gates：标价世代闸表
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS mark_gates (
    id                  VARCHAR(64)   NOT NULL,
    asset               VARCHAR(32)   NOT NULL DEFAULT '',
    pair_asset          VARCHAR(32)   NOT NULL DEFAULT '',
    mark_price          VARCHAR(64)   NOT NULL DEFAULT '0',
    generation          BIGINT        NOT NULL DEFAULT 0,
    observed_at         TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    max_age_seconds     INT           NOT NULL DEFAULT 60,
    poisoned            TINYINT(1)    NOT NULL DEFAULT 0,
    poison_note         VARCHAR(512)  NOT NULL DEFAULT '',
    plan_id             VARCHAR(64)   NOT NULL DEFAULT '',
    created_at          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_mark_gates_asset (asset),
    KEY idx_mark_gates_generation (asset, generation),
    KEY idx_mark_gates_plan (plan_id),
    KEY idx_mark_gates_observed (observed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='标价世代闸记录表';

-- ─────────────────────────────────────────────
-- liq_proofs：强平终态证明表
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS liq_proofs (
    id                  VARCHAR(64)   NOT NULL,
    plan_id             VARCHAR(64)   NOT NULL,
    occupy_token        VARCHAR(64)   NOT NULL DEFAULT '',
    collateral_asset    VARCHAR(32)   NOT NULL DEFAULT '',
    collateral_amount   VARCHAR(64)   NOT NULL DEFAULT '0',
    total_seized        VARCHAR(64)   NOT NULL DEFAULT '0',
    borrow_asset        VARCHAR(32)   NOT NULL DEFAULT '',
    borrow_amount       VARCHAR(64)   NOT NULL DEFAULT '0',
    settled_amount      VARCHAR(64)   NOT NULL DEFAULT '0',
    total_residue       VARCHAR(64)   NOT NULL DEFAULT '0',
    seized_legs_json    MEDIUMTEXT    NOT NULL DEFAULT '[]',
    residue_legs_json   MEDIUMTEXT    NOT NULL DEFAULT '[]',
    digest              VARCHAR(128)  NOT NULL DEFAULT '',
    status              VARCHAR(32)   NOT NULL DEFAULT 'draft',
    validated_by        VARCHAR(256)  NOT NULL DEFAULT '',
    validated_at        TIMESTAMP     NULL,
    created_at          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_liq_proofs_plan (plan_id),
    KEY idx_liq_proofs_status (status),
    KEY idx_liq_proofs_created (created_at),
    CONSTRAINT fk_liq_proofs_plan FOREIGN KEY (plan_id) REFERENCES liq_plans (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='强平终态证明表（不可变）';

-- ─────────────────────────────────────────────
-- occupy_tokens：占用令牌表（防同一 borrow 并发开两个计划）
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS occupy_tokens (
    borrow_id           VARCHAR(64)   NOT NULL,
    token               VARCHAR(64)   NOT NULL DEFAULT '',
    plan_id             VARCHAR(64)   NOT NULL DEFAULT '',
    issued_at           TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (borrow_id),
    KEY idx_occupy_tokens_plan (plan_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='强平占用令牌表（每 borrow 至多一个活跃）';
// temporary guard removed after the production check
