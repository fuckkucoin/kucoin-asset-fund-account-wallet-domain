-- Decide→Exec 决策租约、漂移证明与世代表族。
-- 含：cer_decide_leases、cer_drift_proofs、cer_decide_audit、
--     cer_decide_flip_events、cer_decide_generations。
-- 不含：仪式批次绑定表、HSM 槽表、signpipe 故障注入表。
--
-- 作者：Eco
-- 日期：2025.08.19

-- ── cer_decide_leases ────────────────────────────────────────────────────────
-- 状态：issued → bound → consumed | expired | revoked。
-- 同 lease_id 主键；idempotency_key 唯一。无 DELETE。
CREATE TABLE IF NOT EXISTS cer_decide_leases (
    lease_id            VARCHAR(36)     NOT NULL,
    batch_id            VARCHAR(64)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    chain               VARCHAR(16)     NOT NULL,
    amount              DECIMAL(36,18)  NOT NULL,
    dest                VARCHAR(191)    NOT NULL,
    dest_risk           VARCHAR(16)     NOT NULL DEFAULT '',
    side                VARCHAR(16)     NOT NULL DEFAULT '',
    purpose             VARCHAR(32)     NOT NULL DEFAULT '',
    policy_id           VARCHAR(64)     NOT NULL DEFAULT '',
    threshold_n         INT             NOT NULL DEFAULT 0,
    shares_n            INT             NOT NULL DEFAULT 0,
    bind_mode           VARCHAR(16)     NOT NULL DEFAULT '',
    required_flag       TINYINT         NOT NULL DEFAULT 1,
    state               VARCHAR(16)     NOT NULL DEFAULT 'issued',
    ttl_sec             BIGINT          NOT NULL DEFAULT 900,
    idempotency_key     VARCHAR(128)    NOT NULL,
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    generation          BIGINT          NOT NULL DEFAULT 1,
    flip_epoch          BIGINT          NOT NULL DEFAULT 1,
    exec_ref            VARCHAR(128)    NOT NULL DEFAULT '',
    issued_at           DATETIME(3)     NOT NULL,
    bound_at            DATETIME(3)     NULL,
    consumed_at         DATETIME(3)     NULL,
    expired_at          DATETIME(3)     NULL,
    revoked_at          DATETIME(3)     NULL,
    revoke_why          VARCHAR(128)    NOT NULL DEFAULT '',
    pack_digest         VARCHAR(64)     NOT NULL DEFAULT '',
    line_digest         VARCHAR(64)     NOT NULL DEFAULT '',
    snap_digest         VARCHAR(64)     NOT NULL DEFAULT '',
    fp_digest           VARCHAR(64)     NOT NULL DEFAULT '',
    force_cold          TINYINT         NOT NULL DEFAULT 1,
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (lease_id),
    UNIQUE KEY uq_cdl_idem (idempotency_key),
    KEY idx_cdl_batch_state (batch_id, state),
    KEY idx_cdl_policy_state (policy_id, state),
    KEY idx_cdl_state_issued (state, issued_at),
    KEY idx_cdl_asset_chain (asset, chain)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='冷签决策租约 decide_lease';

-- ── cer_drift_proofs ─────────────────────────────────────────────────────────
-- 拒执行/漂移不可变证明。INSERT IGNORE；无 UPDATE/DELETE。
CREATE TABLE IF NOT EXISTS cer_drift_proofs (
    proof_id            VARCHAR(36)     NOT NULL,
    lease_id            VARCHAR(36)     NOT NULL DEFAULT '',
    batch_id            VARCHAR(64)     NOT NULL DEFAULT '',
    kind                VARCHAR(24)     NOT NULL,
    quoted_policy       VARCHAR(64)     NOT NULL DEFAULT '',
    live_policy         VARCHAR(64)     NOT NULL DEFAULT '',
    quoted_thresh       INT             NOT NULL DEFAULT 0,
    live_thresh         INT             NOT NULL DEFAULT 0,
    quoted_shares       INT             NOT NULL DEFAULT 0,
    live_shares         INT             NOT NULL DEFAULT 0,
    quoted_risk         VARCHAR(16)     NOT NULL DEFAULT '',
    live_risk           VARCHAR(16)     NOT NULL DEFAULT '',
    quoted_force        TINYINT         NOT NULL DEFAULT 0,
    live_force          TINYINT         NOT NULL DEFAULT 0,
    quoted_gen          BIGINT          NOT NULL DEFAULT 0,
    live_gen            BIGINT          NOT NULL DEFAULT 0,
    quoted_digest       VARCHAR(64)     NOT NULL DEFAULT '',
    live_digest         VARCHAR(64)     NOT NULL DEFAULT '',
    detail              VARCHAR(512)    NOT NULL DEFAULT '',
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    at_ts               DATETIME(3)     NOT NULL,
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (proof_id),
    KEY idx_cdp_lease (lease_id),
    KEY idx_cdp_batch (batch_id),
    KEY idx_cdp_kind (kind, at_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='冷签决策漂移证明 CerDriftProof';

-- ── cer_decide_audit ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS cer_decide_audit (
    audit_id            VARCHAR(36)     NOT NULL,
    lease_id            VARCHAR(36)     NOT NULL,
    batch_id            VARCHAR(64)     NOT NULL DEFAULT '',
    action              VARCHAR(32)     NOT NULL,
    from_state          VARCHAR(16)     NOT NULL DEFAULT '',
    to_state            VARCHAR(16)     NOT NULL DEFAULT '',
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    detail              VARCHAR(256)    NOT NULL DEFAULT '',
    at_ts               DATETIME(3)     NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_cda_lease_at (lease_id, at_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='冷签决策租约审计';

-- ── cer_decide_flip_events ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS cer_decide_flip_events (
    event_id            VARCHAR(36)     NOT NULL,
    scope_key           VARCHAR(128)    NOT NULL,
    kind                VARCHAR(24)     NOT NULL,
    policy_id           VARCHAR(64)     NOT NULL DEFAULT '',
    old_force           TINYINT         NOT NULL DEFAULT 0,
    new_force           TINYINT         NOT NULL DEFAULT 0,
    old_thresh          INT             NOT NULL DEFAULT 0,
    new_thresh          INT             NOT NULL DEFAULT 0,
    old_shares          INT             NOT NULL DEFAULT 0,
    new_shares          INT             NOT NULL DEFAULT 0,
    old_risk            VARCHAR(16)     NOT NULL DEFAULT '',
    new_risk            VARCHAR(16)     NOT NULL DEFAULT '',
    epoch               BIGINT          NOT NULL DEFAULT 0,
    at_ts               DATETIME(3)     NOT NULL,
    done_flag           TINYINT         NOT NULL DEFAULT 0,
    PRIMARY KEY (event_id),
    KEY idx_cdfe_done (done_flag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='策略包与风险档翻转事件';

-- ── cer_decide_generations ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS cer_decide_generations (
    scope_key           VARCHAR(128)    NOT NULL,
    epoch               BIGINT          NOT NULL DEFAULT 0,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (scope_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='Decide 租约世代计数';
