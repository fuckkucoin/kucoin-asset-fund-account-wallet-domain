-- 对账单导出过期与校验根漂移封堵引擎 DDL
--
-- 作者：TianpengLan
-- 日期：2021.08.15

CREATE TABLE IF NOT EXISTS stmt_export_leases (
    id           VARCHAR(64)  PRIMARY KEY,
    user_id      VARCHAR(64)  NOT NULL,
    format       VARCHAR(16)  NOT NULL DEFAULT 'json',
    root         VARCHAR(128) NOT NULL,
    high_water   INT          NOT NULL DEFAULT 0,
    state        VARCHAR(16)  NOT NULL DEFAULT 'built',
    ttl_sec      INT          NOT NULL DEFAULT 3600,
    issued_at    DATETIME     NULL,
    expires_at   DATETIME     NULL,
    created_at   DATETIME     NOT NULL,
    updated_at   DATETIME     NOT NULL,
    window_start DATETIME     NULL,
    window_end   DATETIME     NULL,
    revoked_by   VARCHAR(256) NULL,
    proof_id     VARCHAR(64)  NULL,
    INDEX idx_sel_user_state (user_id, state),
    INDEX idx_sel_state_expires (state, expires_at),
    INDEX idx_sel_state_issued (state, issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS stmt_root_drifts (
    id              VARCHAR(64)  PRIMARY KEY,
    lease_id        VARCHAR(64)  NOT NULL,
    user_id         VARCHAR(64)  NOT NULL,
    original_root   VARCHAR(128) NOT NULL,
    current_root    VARCHAR(128) NOT NULL,
    high_water_old  INT          NOT NULL DEFAULT 0,
    high_water_new  INT          NOT NULL DEFAULT 0,
    drift_bill_seqs TEXT         NULL,
    state           VARCHAR(16)  NOT NULL DEFAULT 'open',
    created_at      DATETIME     NOT NULL,
    proven_at       DATETIME     NULL,
    closed_at       DATETIME     NULL,
    resolution      VARCHAR(512) NULL,
    INDEX idx_srd_lease (lease_id),
    INDEX idx_srd_state (state, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS stmt_revocations (
    id         VARCHAR(64)  PRIMARY KEY,
    lease_id   VARCHAR(64)  NOT NULL,
    user_id    VARCHAR(64)  NOT NULL,
    kind       VARCHAR(32)  NOT NULL,
    reason     VARCHAR(512) NOT NULL DEFAULT '',
    drift_id   VARCHAR(64)  NULL,
    created_at DATETIME     NOT NULL,
    actor      VARCHAR(128) NOT NULL DEFAULT '',
    INDEX idx_sr_lease (lease_id),
    INDEX idx_sr_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS stmt_issue_proofs (
    id           VARCHAR(64)  PRIMARY KEY,
    lease_id     VARCHAR(64)  NOT NULL,
    user_id      VARCHAR(64)  NOT NULL,
    root         VARCHAR(128) NOT NULL,
    ttl_sec      INT          NOT NULL,
    digest       VARCHAR(128) NOT NULL,
    issued_at    DATETIME     NOT NULL,
    expires_at   DATETIME     NOT NULL,
    window_start DATETIME     NOT NULL,
    window_end   DATETIME     NOT NULL,
    INDEX idx_sip_lease (lease_id),
    INDEX idx_sip_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
