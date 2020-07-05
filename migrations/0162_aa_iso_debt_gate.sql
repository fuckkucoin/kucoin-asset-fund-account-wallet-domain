-- Migration 0052：逐仓科目借贷比漂移与转入闸竞态封堵
-- 表：iso_pair_generations, iso_ratio_leases, iso_transfer_gates,
--      iso_ratio_drifts, iso_drift_proofs, iso_mutex_records
-- 作者：Lennox
-- 日期：2020.12.11

-- ─────────────────────────────────────────────
-- iso_pair_generations：Pair 借贷比世代历史
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS iso_pair_generations (
    id              VARCHAR(64)    NOT NULL,
    user_id         VARCHAR(128)   NOT NULL DEFAULT '',
    symbol          VARCHAR(32)    NOT NULL DEFAULT '',
    generation      BIGINT         NOT NULL DEFAULT 0,
    debt_ratio      VARCHAR(64)    NOT NULL DEFAULT '0',
    total_debt      VARCHAR(64)    NOT NULL DEFAULT '0',
    total_equity    VARCHAR(64)    NOT NULL DEFAULT '0',
    computed_at     TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    triggered_by    VARCHAR(32)    NOT NULL DEFAULT '',   -- credit | debit | recompute
    source_txn_id   VARCHAR(128)   NOT NULL DEFAULT '',
    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_iso_pair_gen_user_sym_gen
    ON iso_pair_generations (user_id, symbol, generation);

CREATE INDEX IF NOT EXISTS idx_iso_pair_gen_user_sym
    ON iso_pair_generations (user_id, symbol, computed_at DESC);

-- ─────────────────────────────────────────────
-- iso_ratio_leases：比率租约主表
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS iso_ratio_leases (
    id              VARCHAR(64)    NOT NULL,
    user_id         VARCHAR(128)   NOT NULL DEFAULT '',
    symbol          VARCHAR(32)    NOT NULL DEFAULT '',
    generation      BIGINT         NOT NULL DEFAULT 0,
    debt_ratio      VARCHAR(64)    NOT NULL DEFAULT '0',
    max_ratio       VARCHAR(64)    NOT NULL DEFAULT '0',
    state           VARCHAR(16)    NOT NULL DEFAULT 'fresh',  -- fresh | stale | blocked
    issued_at       TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at      TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    consumed_at     TIMESTAMP      NULL,
    staled_at       TIMESTAMP      NULL,
    blocked_at      TIMESTAMP      NULL,
    block_reason    VARCHAR(512)   NOT NULL DEFAULT '',
    issued_by       VARCHAR(128)   NOT NULL DEFAULT '',
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_iso_ratio_leases_user_sym_state
    ON iso_ratio_leases (user_id, symbol, state, expires_at);

CREATE INDEX IF NOT EXISTS idx_iso_ratio_leases_state
    ON iso_ratio_leases (state);

-- ─────────────────────────────────────────────
-- iso_transfer_gates：转入裁决记录
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS iso_transfer_gates (
    id              VARCHAR(64)    NOT NULL,
    lease_id        VARCHAR(64)    NOT NULL DEFAULT '',
    user_id         VARCHAR(128)   NOT NULL DEFAULT '',
    symbol          VARCHAR(32)    NOT NULL DEFAULT '',
    currency        VARCHAR(32)    NOT NULL DEFAULT '',
    amount          VARCHAR(64)    NOT NULL DEFAULT '0',
    state           VARCHAR(16)    NOT NULL DEFAULT 'pending',  -- pending | allowed | rejected
    reject_reason   VARCHAR(512)   NOT NULL DEFAULT '',
    created_at      TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    adjudicated_at  TIMESTAMP      NULL,
    requested_by    VARCHAR(128)   NOT NULL DEFAULT '',
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_iso_transfer_gates_lease
    ON iso_transfer_gates (lease_id);

CREATE INDEX IF NOT EXISTS idx_iso_transfer_gates_user_sym
    ON iso_transfer_gates (user_id, symbol, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_iso_transfer_gates_state
    ON iso_transfer_gates (state);

-- ─────────────────────────────────────────────
-- iso_ratio_drifts：比率漂移案例
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS iso_ratio_drifts (
    id               VARCHAR(64)    NOT NULL,
    user_id          VARCHAR(128)   NOT NULL DEFAULT '',
    symbol           VARCHAR(32)    NOT NULL DEFAULT '',
    displayed_ratio  VARCHAR(64)    NOT NULL DEFAULT '0',
    actual_debt      VARCHAR(64)    NOT NULL DEFAULT '0',
    actual_equity    VARCHAR(64)    NOT NULL DEFAULT '0',
    actual_ratio     VARCHAR(64)    NOT NULL DEFAULT '0',
    generation       BIGINT         NOT NULL DEFAULT 0,
    state            VARCHAR(16)    NOT NULL DEFAULT 'open',  -- open | proven | closed
    detected_at      TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    proven_at        TIMESTAMP      NULL,
    closed_at        TIMESTAMP      NULL,
    proof_id         VARCHAR(64)    NOT NULL DEFAULT '',
    proof_summary    VARCHAR(1024)  NOT NULL DEFAULT '',
    closed_by        VARCHAR(128)   NOT NULL DEFAULT '',
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_iso_ratio_drifts_user_sym_state
    ON iso_ratio_drifts (user_id, symbol, state);

CREATE INDEX IF NOT EXISTS idx_iso_ratio_drifts_state
    ON iso_ratio_drifts (state, detected_at DESC);

-- ─────────────────────────────────────────────
-- iso_drift_proofs：漂移证明
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS iso_drift_proofs (
    id              VARCHAR(64)    NOT NULL,
    drift_case_id   VARCHAR(64)    NOT NULL DEFAULT '',
    submitted_by    VARCHAR(128)   NOT NULL DEFAULT '',
    summary         VARCHAR(1024)  NOT NULL DEFAULT '',
    evidence        TEXT           NOT NULL DEFAULT '',
    submitted_at    TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    confirmed_by    VARCHAR(128)   NOT NULL DEFAULT '',
    confirmed_at    TIMESTAMP      NULL,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_iso_drift_proofs_case
    ON iso_drift_proofs (drift_case_id);

-- ─────────────────────────────────────────────
-- iso_mutex_records：借记↔转入冲突记录
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS iso_mutex_records (
    id              VARCHAR(64)    NOT NULL,
    kind            VARCHAR(32)    NOT NULL DEFAULT 'debit_vs_transfer',
    user_id         VARCHAR(128)   NOT NULL DEFAULT '',
    symbol          VARCHAR(32)    NOT NULL DEFAULT '',
    debit_txn_id    VARCHAR(128)   NOT NULL DEFAULT '',
    transfer_id     VARCHAR(128)   NOT NULL DEFAULT '',
    detected_at     TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolution      VARCHAR(64)    NOT NULL DEFAULT '',
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_iso_mutex_records_user_sym
    ON iso_mutex_records (user_id, symbol, detected_at DESC);
