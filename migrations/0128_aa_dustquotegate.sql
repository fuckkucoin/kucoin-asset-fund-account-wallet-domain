-- 尘兑路径牌价租约、漂移证明与审计表族。
-- 含：dust_quote_leases、dust_quote_drift_proofs、dust_quote_audit、
--     dust_quote_scan_cursors、dust_quote_flip_events、dust_quote_flip_epochs。
-- 不含：dustleak 超额公式表、cvfund 闪兑租约表、feeeng 档位表。
--
-- 作者：Eco
-- 日期：2025.08.19

-- ── dust_quote_leases ────────────────────────────────────────────────────────
-- 状态：quoted → bound → settled | expired | revoked。
-- 同 lease_id 主键；idempotency_key 唯一。无 DELETE。
CREATE TABLE IF NOT EXISTS dust_quote_leases (
    lease_id            VARCHAR(36)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    amount              DECIMAL(36,18)  NOT NULL,
    usdt_want           DECIMAL(36,18)  NOT NULL,
    rate                VARCHAR(64)     NOT NULL,
    policy_gen          BIGINT          NOT NULL DEFAULT 1,
    cap_bps             BIGINT          NOT NULL DEFAULT 50,
    book_fp             VARCHAR(64)     NOT NULL DEFAULT '',
    digest              VARCHAR(64)     NOT NULL DEFAULT '',
    state               VARCHAR(16)     NOT NULL DEFAULT 'quoted',
    ttl_sec             BIGINT          NOT NULL DEFAULT 120,
    idempotency_key     VARCHAR(128)    NOT NULL,
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    scan_batch          VARCHAR(128)    NOT NULL DEFAULT '',
    scan_index          INT             NOT NULL DEFAULT 0,
    quoted_at           DATETIME(3)     NOT NULL,
    bound_at            DATETIME(3)     NULL,
    settled_at          DATETIME(3)     NULL,
    expired_at          DATETIME(3)     NULL,
    revoked_at          DATETIME(3)     NULL,
    revoke_why          VARCHAR(128)    NOT NULL DEFAULT '',
    flip_epoch          BIGINT          NOT NULL DEFAULT 0,
    convert_id          VARCHAR(64)     NOT NULL DEFAULT '',
    floor_pinned        VARCHAR(64)     NOT NULL DEFAULT '',
    min_pinned          VARCHAR(64)     NOT NULL DEFAULT '',
    book_avail          VARCHAR(64)     NOT NULL DEFAULT '',
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (lease_id),
    UNIQUE KEY uq_dql_idem (idempotency_key),
    KEY idx_dql_user_asset (user_id, asset),
    KEY idx_dql_state_quoted (state, quoted_at),
    KEY idx_dql_asset_state (asset, state),
    KEY idx_dql_scan_batch (scan_batch)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='尘兑牌价租约 dust_quote_lease';

-- ── dust_quote_drift_proofs ──────────────────────────────────────────────────
-- 拒兑/漂移不可变证明。INSERT IGNORE；无 UPDATE/DELETE。
CREATE TABLE IF NOT EXISTS dust_quote_drift_proofs (
    proof_id            VARCHAR(36)     NOT NULL,
    lease_id            VARCHAR(36)     NOT NULL DEFAULT '',
    user_id             VARCHAR(64)     NOT NULL DEFAULT '',
    asset               VARCHAR(16)     NOT NULL DEFAULT '',
    kind                VARCHAR(24)     NOT NULL,
    quoted_rate         VARCHAR(64)     NOT NULL DEFAULT '',
    live_rate           VARCHAR(64)     NOT NULL DEFAULT '',
    quoted_gen          BIGINT          NOT NULL DEFAULT 0,
    live_gen            BIGINT          NOT NULL DEFAULT 0,
    quoted_book         VARCHAR(64)     NOT NULL DEFAULT '',
    live_book           VARCHAR(64)     NOT NULL DEFAULT '',
    quoted_cap          BIGINT          NOT NULL DEFAULT 0,
    live_cap            BIGINT          NOT NULL DEFAULT 0,
    detail              VARCHAR(512)    NOT NULL DEFAULT '',
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    scan_batch          VARCHAR(128)    NOT NULL DEFAULT '',
    at                  DATETIME(3)     NOT NULL,
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (proof_id),
    KEY idx_dqdp_lease (lease_id),
    KEY idx_dqdp_user (user_id),
    KEY idx_dqdp_kind (kind, at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='尘兑漂移拒兑证明 DustQuoteDriftProof';

-- ── dust_quote_audit ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS dust_quote_audit (
    audit_id            VARCHAR(36)     NOT NULL,
    lease_id            VARCHAR(36)     NOT NULL,
    action              VARCHAR(32)     NOT NULL,
    from_state          VARCHAR(16)     NOT NULL DEFAULT '',
    to_state            VARCHAR(16)     NOT NULL DEFAULT '',
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    detail              VARCHAR(256)    NOT NULL DEFAULT '',
    at                  DATETIME(3)     NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_dqa_lease_at (lease_id, at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='尘兑牌价租约审计';

-- ── dust_quote_scan_cursors ──────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS dust_quote_scan_cursors (
    batch_id            VARCHAR(128)    NOT NULL,
    user_id             VARCHAR(64)     NOT NULL,
    next_index          INT             NOT NULL DEFAULT 0,
    halted              TINYINT         NOT NULL DEFAULT 0,
    halt_reason         VARCHAR(256)    NOT NULL DEFAULT '',
    settled_n           INT             NOT NULL DEFAULT 0,
    rejected_n          INT             NOT NULL DEFAULT 0,
    flip_epoch          BIGINT          NOT NULL DEFAULT 0,
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (batch_id),
    KEY idx_dqsc_halted (halted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='ConvertScan 中断游标';

-- ── dust_quote_flip_events ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS dust_quote_flip_events (
    event_id            VARCHAR(36)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    kind                VARCHAR(24)     NOT NULL,
    old_rate            VARCHAR(64)     NOT NULL DEFAULT '',
    new_rate            VARCHAR(64)     NOT NULL DEFAULT '',
    old_gen             BIGINT          NOT NULL DEFAULT 0,
    new_gen             BIGINT          NOT NULL DEFAULT 0,
    epoch               BIGINT          NOT NULL DEFAULT 0,
    at                  DATETIME(3)     NOT NULL,
    done                TINYINT         NOT NULL DEFAULT 0,
    PRIMARY KEY (event_id),
    KEY idx_dqfe_done (done)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='牌价/策略翻转事件';

-- ── dust_quote_flip_epochs ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS dust_quote_flip_epochs (
    asset               VARCHAR(16)     NOT NULL,
    epoch               BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='资产翻转世代计数';
