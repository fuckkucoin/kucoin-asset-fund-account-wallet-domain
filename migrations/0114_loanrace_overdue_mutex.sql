-- 借贷逾期冻结与补还核销竞态收口：DDL
-- 作者：LoanRace-Module
-- 日期：2024.12

-- ─────────────────────────────────────
-- 逾期冻结租约表
-- ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS loan_overdue_leases (
    id            VARCHAR(36)  NOT NULL PRIMARY KEY,
    loan_id       VARCHAR(36)  NOT NULL,
    user_id       VARCHAR(36)  NOT NULL,
    generation    BIGINT       NOT NULL DEFAULT 0,
    state         VARCHAR(20)  NOT NULL DEFAULT 'acquired',
    holder        VARCHAR(64)  NOT NULL,
    acquired_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    applied_at    TIMESTAMP    NULL,
    released_at   TIMESTAMP    NULL,
    expires_at    TIMESTAMP    NOT NULL,
    reason        TEXT,
    INDEX idx_lease_loan_id (loan_id),
    INDEX idx_lease_state (state),
    INDEX idx_lease_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ─────────────────────────────────────
-- 还款互斥门表
-- ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS pay_gates (
    id            VARCHAR(36)  NOT NULL PRIMARY KEY,
    loan_id       VARCHAR(36)  NOT NULL,
    user_id       VARCHAR(36)  NOT NULL,
    lease_id      VARCHAR(36)  NOT NULL,
    state         VARCHAR(20)  NOT NULL DEFAULT 'open',
    allowed_amt   VARCHAR(64)  NOT NULL,
    paid_amt      VARCHAR(64)  NOT NULL DEFAULT '0',
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cleared_at    TIMESTAMP    NULL,
    closed_at     TIMESTAMP    NULL,
    actor         VARCHAR(64)  NOT NULL,
    INDEX idx_gate_loan_id (loan_id),
    INDEX idx_gate_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ─────────────────────────────────────
-- 核销工单表
-- ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS writeoff_cases (
    id                VARCHAR(36)  NOT NULL PRIMARY KEY,
    loan_id           VARCHAR(36)  NOT NULL UNIQUE,
    user_id           VARCHAR(36)  NOT NULL,
    principal         VARCHAR(64)  NOT NULL,
    interest          VARCHAR(64)  NOT NULL,
    suspense_amt      VARCHAR(64)  NOT NULL,
    state             VARCHAR(20)  NOT NULL DEFAULT 'pending',
    applicant_actor   VARCHAR(64)  NOT NULL,
    approver_actor    VARCHAR(64),
    reason            TEXT         NOT NULL,
    created_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    approved_at       TIMESTAMP    NULL,
    applied_at        TIMESTAMP    NULL,
    rejected_at       TIMESTAMP    NULL,
    INDEX idx_wo_state (state),
    INDEX idx_wo_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ─────────────────────────────────────
-- 解冻证明表
-- ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS unfreeze_proofs (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY,
    loan_id         VARCHAR(36)  NOT NULL,
    user_id         VARCHAR(36)  NOT NULL,
    kind            VARCHAR(32)  NOT NULL,
    evidence_ref    TEXT         NOT NULL,
    evidence_hash   VARCHAR(64)  NOT NULL,
    state           VARCHAR(20)  NOT NULL DEFAULT 'pending',
    submitted_by    VARCHAR(64)  NOT NULL,
    validated_by    VARCHAR(64),
    submitted_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    validated_at    TIMESTAMP    NULL,
    note            TEXT,
    INDEX idx_proof_loan_id (loan_id),
    INDEX idx_proof_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ─────────────────────────────────────
-- 呆滞科目条目表
-- ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS suspense_entries (
    id            VARCHAR(36)  NOT NULL PRIMARY KEY,
    loan_id       VARCHAR(36)  NOT NULL,
    user_id       VARCHAR(36)  NOT NULL,
    amount        VARCHAR(64)  NOT NULL,
    kind          VARCHAR(32)  NOT NULL,
    ref           VARCHAR(36)  NOT NULL,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_suspense_loan_id (loan_id),
    INDEX idx_suspense_kind (kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
