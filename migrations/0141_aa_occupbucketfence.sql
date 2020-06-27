-- 充值占用三桶围栏与守恒击穿证明表族。
-- 含：dep_occup_lots / holds / releases / reclasses / audits、
--     dep_occup_bucket_fences、dep_occup_break_proofs、dep_occup_half_cases、dep_occup_lot_mutex。
-- 不含：limitgate/usage 租约、r4 延迟占用比例参数表。
--
-- 作者：Eco
-- 日期：2025.11.18

CREATE TABLE IF NOT EXISTS dep_occup_lots (
    lot_id              VARCHAR(64)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    occupied_total      DECIMAL(36,18)  NOT NULL,
    observed_amt        DECIMAL(36,18)  NOT NULL,
    risk_class          VARCHAR(16)     NOT NULL DEFAULT 'normal',
    state               VARCHAR(16)     NOT NULL DEFAULT 'observed',
    inflight            DECIMAL(36,18)  NOT NULL DEFAULT 0,
    available_amt       DECIMAL(36,18)  NOT NULL DEFAULT 0,
    void_amt            DECIMAL(36,18)  NOT NULL DEFAULT 0,
    observe_ref         VARCHAR(128)    NOT NULL DEFAULT '',
    half_open           TINYINT(1)      NOT NULL DEFAULT 0,
    generation          BIGINT          NOT NULL DEFAULT 1,
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    closed_at           DATETIME(3)     NULL,
    PRIMARY KEY (lot_id),
    KEY idx_dol_user (user_id),
    KEY idx_dol_state (state, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='充值占用批次三桶';

CREATE TABLE IF NOT EXISTS dep_occup_holds (
    hold_id             VARCHAR(64)     NOT NULL,
    lot_id              VARCHAR(64)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    amount              DECIMAL(36,18)  NOT NULL,
    idem_key            VARCHAR(128)    NOT NULL DEFAULT '',
    risk_class          VARCHAR(16)     NOT NULL DEFAULT 'normal',
    released            TINYINT(1)      NOT NULL DEFAULT 0,
    reclassed           TINYINT(1)      NOT NULL DEFAULT 0,
    persist_ok          TINYINT(1)      NOT NULL DEFAULT 1,
    created_at          DATETIME(3)     NOT NULL,
    released_at         DATETIME(3)     NULL,
    PRIMARY KEY (hold_id),
    UNIQUE KEY uq_doh_lot_idem (lot_id, idem_key),
    KEY idx_doh_lot (lot_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='在途 Hold';

CREATE TABLE IF NOT EXISTS dep_occup_releases (
    release_id          VARCHAR(64)     NOT NULL,
    hold_id             VARCHAR(64)     NOT NULL,
    lot_id              VARCHAR(64)     NOT NULL,
    amount              DECIMAL(36,18)  NOT NULL,
    idem_key            VARCHAR(128)    NOT NULL DEFAULT '',
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (release_id),
    UNIQUE KEY uq_dor_lot_idem (lot_id, idem_key),
    KEY idx_dor_hold (hold_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='确认 Release';

CREATE TABLE IF NOT EXISTS dep_occup_reclasses (
    reclass_id          VARCHAR(64)     NOT NULL,
    hold_id             VARCHAR(64)     NOT NULL,
    lot_id              VARCHAR(64)     NOT NULL,
    from_class          VARCHAR(16)     NOT NULL,
    to_class            VARCHAR(16)     NOT NULL,
    amount              DECIMAL(36,18)  NOT NULL,
    idem_key            VARCHAR(128)    NOT NULL DEFAULT '',
    reason_code         VARCHAR(64)     NOT NULL DEFAULT '',
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (reclass_id),
    UNIQUE KEY uq_dorc_lot_idem (lot_id, idem_key),
    KEY idx_dorc_hold (hold_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='升档 Reclass';

CREATE TABLE IF NOT EXISTS dep_occup_audits (
    audit_id            VARCHAR(64)     NOT NULL,
    lot_id              VARCHAR(64)     NOT NULL,
    action              VARCHAR(32)     NOT NULL,
    reason              VARCHAR(64)     NOT NULL,
    ok                  TINYINT(1)      NOT NULL,
    inflight            VARCHAR(64)     NOT NULL,
    available_amt       VARCHAR(64)     NOT NULL,
    void_amt            VARCHAR(64)     NOT NULL,
    total_amt           VARCHAR(64)     NOT NULL,
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_doa_lot (lot_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='占用桶审计';

CREATE TABLE IF NOT EXISTS dep_occup_bucket_fences (
    fence_id            VARCHAR(36)     NOT NULL,
    lot_id              VARCHAR(64)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL DEFAULT '',
    asset               VARCHAR(16)     NOT NULL DEFAULT '',
    bucket_fingerprint  VARCHAR(64)     NOT NULL DEFAULT '',
    generation          BIGINT          NOT NULL DEFAULT 0,
    ttl_seconds         BIGINT          NOT NULL DEFAULT 120,
    state               VARCHAR(16)     NOT NULL DEFAULT 'issued',
    bound_hold_id       VARCHAR(64)     NOT NULL DEFAULT '',
    consumed_by         VARCHAR(16)     NOT NULL DEFAULT '',
    issued_at           DATETIME(3)     NOT NULL,
    bound_at            DATETIME(3)     NULL,
    consumed_at         DATETIME(3)     NULL,
    expires_at          DATETIME(3)     NOT NULL,
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (fence_id),
    KEY idx_dobf_lot_state (lot_id, state),
    KEY idx_dobf_exp (state, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='lot 级 bucket_fence';

CREATE TABLE IF NOT EXISTS dep_occup_break_proofs (
    proof_id            VARCHAR(36)     NOT NULL,
    lot_id              VARCHAR(64)     NOT NULL,
    fence_id            VARCHAR(36)     NOT NULL DEFAULT '',
    state               VARCHAR(16)     NOT NULL DEFAULT 'open',
    kind                VARCHAR(32)     NOT NULL,
    inflight            VARCHAR(64)     NOT NULL,
    available           VARCHAR(64)     NOT NULL,
    void_amt            VARCHAR(64)     NOT NULL,
    sum_amt             VARCHAR(64)     NOT NULL,
    total_amt           VARCHAR(64)     NOT NULL,
    delta_amt           VARCHAR(64)     NOT NULL,
    conflict_op         VARCHAR(16)     NOT NULL DEFAULT '',
    winner_op           VARCHAR(16)     NOT NULL DEFAULT '',
    reason              VARCHAR(256)    NOT NULL DEFAULT '',
    digest              VARCHAR(64)     NOT NULL DEFAULT '',
    opened_at           DATETIME(3)     NOT NULL,
    proven_at           DATETIME(3)     NULL,
    closed_at           DATETIME(3)     NULL,
    PRIMARY KEY (proof_id),
    KEY idx_dobp_lot (lot_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='OccupancyBreakProof';

CREATE TABLE IF NOT EXISTS dep_occup_half_cases (
    case_id             VARCHAR(36)     NOT NULL,
    lot_id              VARCHAR(64)     NOT NULL,
    hold_id             VARCHAR(64)     NOT NULL,
    fence_id            VARCHAR(36)     NOT NULL DEFAULT '',
    state               VARCHAR(16)     NOT NULL DEFAULT 'open',
    amount              VARCHAR(64)     NOT NULL,
    reason              VARCHAR(64)     NOT NULL,
    opened_at           DATETIME(3)     NOT NULL,
    resumed_at          DATETIME(3)     NULL,
    PRIMARY KEY (case_id),
    KEY idx_dohc_lot (lot_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='半截桶续提案';

CREATE TABLE IF NOT EXISTS dep_occup_lot_mutex (
    lot_id              VARCHAR(64)     NOT NULL,
    op_name             VARCHAR(16)     NOT NULL,
    locked_at           DATETIME(3)     NOT NULL,
    PRIMARY KEY (lot_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Release/Reclass 互斥令牌';
