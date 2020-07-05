-- Migration 0051：hotbreach 热库存边界击穿与出金预检竞态封堵
-- 表：hot_pre_leases, hot_breach_cases, hot_breach_proofs
-- 作者：Lennox
-- 日期：2020.12.15

-- ─────────────────────────────────────────────
-- hot_pre_leases：预检世代租约主表
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS hot_pre_leases (
    id                  VARCHAR(64)   NOT NULL,
    asset               VARCHAR(32)   NOT NULL DEFAULT '',
    generation          BIGINT        NOT NULL DEFAULT 0,
    snapshot_hot        VARCHAR(64)   NOT NULL DEFAULT '0',
    snapshot_cap        VARCHAR(64)   NOT NULL DEFAULT '0',
    issued_by           VARCHAR(128)  NOT NULL DEFAULT '',
    state               VARCHAR(32)   NOT NULL DEFAULT 'issued',
    issued_at           TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    consumed_at         TIMESTAMP     NULL,
    consumed_by         VARCHAR(128)  NOT NULL DEFAULT '',
    revoked_at          TIMESTAMP     NULL,
    revoke_reason       VARCHAR(512)  NOT NULL DEFAULT '',
    fence_epoch         BIGINT        NOT NULL DEFAULT 0,
    fence_token         VARCHAR(64)   NOT NULL DEFAULT '',
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_hot_pre_leases_asset_state
    ON hot_pre_leases (asset, state);

CREATE INDEX IF NOT EXISTS idx_hot_pre_leases_expires_at
    ON hot_pre_leases (expires_at)
    WHERE state = 'issued';

CREATE INDEX IF NOT EXISTS idx_hot_pre_leases_issued_by
    ON hot_pre_leases (issued_by);

-- ─────────────────────────────────────────────
-- hot_breach_cases：击穿案例主表
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS hot_breach_cases (
    id                      VARCHAR(64)   NOT NULL,
    asset                   VARCHAR(32)   NOT NULL DEFAULT '',
    lease_id                VARCHAR(64)   NOT NULL DEFAULT '',
    pre_check_hot           VARCHAR(64)   NOT NULL DEFAULT '0',
    breach_hot              VARCHAR(64)   NOT NULL DEFAULT '0',
    cap_value               VARCHAR(64)   NOT NULL DEFAULT '0',
    excess                  VARCHAR(64)   NOT NULL DEFAULT '0',
    concurrent_party        VARCHAR(128)  NOT NULL DEFAULT '',
    concurrent_party_hash   VARCHAR(64)   NOT NULL DEFAULT '',
    state                   VARCHAR(32)   NOT NULL DEFAULT 'open',
    detected_at             TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    gated_at                TIMESTAMP     NULL,
    cleared_at              TIMESTAMP     NULL,
    cleared_by              VARCHAR(256)  NOT NULL DEFAULT '',
    clear_note              VARCHAR(1024) NOT NULL DEFAULT '',
    proof_id                VARCHAR(64)   NOT NULL DEFAULT '',
    fence_epoch             BIGINT        NOT NULL DEFAULT 0,
    fence_token             VARCHAR(64)   NOT NULL DEFAULT '',
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_hot_breach_cases_asset_state
    ON hot_breach_cases (asset, state);

CREATE INDEX IF NOT EXISTS idx_hot_breach_cases_lease_id
    ON hot_breach_cases (lease_id);

CREATE INDEX IF NOT EXISTS idx_hot_breach_cases_detected_at
    ON hot_breach_cases (detected_at DESC);

-- ─────────────────────────────────────────────
-- hot_breach_proofs：击穿证明表
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS hot_breach_proofs (
    id                      VARCHAR(64)     NOT NULL,
    breach_id               VARCHAR(64)     NOT NULL DEFAULT '',
    asset                   VARCHAR(32)     NOT NULL DEFAULT '',
    snapshot_digest         VARCHAR(128)    NOT NULL DEFAULT '',
    lease_digest            VARCHAR(128)    NOT NULL DEFAULT '',
    concurrent_party_hash   VARCHAR(64)     NOT NULL DEFAULT '',
    composite_hash          VARCHAR(128)    NOT NULL DEFAULT '',
    raw_snapshot_json       TEXT            NOT NULL DEFAULT '',
    raw_lease_json          TEXT            NOT NULL DEFAULT '',
    created_at              TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_hot_breach_proofs_breach_id
    ON hot_breach_proofs (breach_id);

CREATE INDEX IF NOT EXISTS idx_hot_breach_proofs_composite_hash
    ON hot_breach_proofs (composite_hash);
