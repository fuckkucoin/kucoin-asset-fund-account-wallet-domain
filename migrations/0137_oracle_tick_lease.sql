-- 估值 Tick 与资金写路径租约漂移封堵引擎迁移。
--
-- 表：
--   oracle_tick_leases         — Convert/MTM/LTV 签发、资金写原子消费
--   oracle_tick_drift_proofs   — Tick 翻转 / 过期 / Shadow 冒充证明仓
--   oracle_tick_audit          — 估值租约审计
--   oracle_tick_generations    — pair 世代
--   oracle_tick_fund_writes    — 一租约一写，防双花
--   oracle_tick_idem           — 幂等键
--
-- 原则：
--   1. 无租约、过期、缺价不得进入资金写成功路径。
--   2. Tick Put 后旧租约立即吊销并开 TickDriftProof。
--   3. 不引用 pricepoison / feeoracle / fxmark 外键，隔离在应用层。
--
-- 迁移标识：0137
-- 作者：Span
-- 日期：2021.12.18

-- +migrate Up

CREATE TABLE IF NOT EXISTS oracle_tick_leases (
    lease_id         VARCHAR(64)  NOT NULL COMMENT 'tick_lease_id',
    pair             VARCHAR(32)  NOT NULL,
    asset            VARCHAR(16)  NOT NULL,
    quote            VARCHAR(16)  NOT NULL,
    price            VARCHAR(64)  NOT NULL,
    tick_at          DATETIME(3)  NOT NULL,
    fingerprint      VARCHAR(64)  NOT NULL,
    generation       BIGINT       NOT NULL DEFAULT 1,
    valuation_kind   VARCHAR(32)  NOT NULL COMMENT 'convert|mark_to_market|margin_ltv',
    mode             VARCHAR(16)  NOT NULL COMMENT 'enforce|shadow|probe',
    quote_ccy        VARCHAR(16)  NOT NULL,
    amount           VARCHAR(64)  NOT NULL DEFAULT '',
    borrow_asset     VARCHAR(16)  NOT NULL DEFAULT '',
    borrow_amt       VARCHAR(64)  NOT NULL DEFAULT '',
    col_asset        VARCHAR(16)  NOT NULL DEFAULT '',
    col_amt          VARCHAR(64)  NOT NULL DEFAULT '',
    mtm_total        VARCHAR(64)  NOT NULL DEFAULT '',
    ltv_bps          INT          NOT NULL DEFAULT 0,
    state            VARCHAR(16)  NOT NULL DEFAULT 'issued'
                     COMMENT 'issued|bound|consumed|expired|revoked',
    ttl_seconds      BIGINT       NOT NULL DEFAULT 30,
    issued_at        DATETIME(3)  NOT NULL,
    expires_at       DATETIME(3)  NOT NULL,
    bound_at         DATETIME(3)  NULL,
    KEY idx_otl_state_exp (state, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='估值 Tick 资金写租约';

CREATE TABLE IF NOT EXISTS oracle_tick_drift_proofs (
    proof_id          VARCHAR(64)  NOT NULL,
    lease_id          VARCHAR(64)  NOT NULL DEFAULT '',
    pair              VARCHAR(32)  NOT NULL DEFAULT '',
    kind              VARCHAR(64)  NOT NULL,
    fail_closed       TINYINT(1)   NOT NULL DEFAULT 1,
    issued_price      VARCHAR(64)  NOT NULL DEFAULT '',
    live_price        VARCHAR(64)  NOT NULL DEFAULT '',
    issued_generation BIGINT       NOT NULL DEFAULT 0,
    live_generation   BIGINT       NOT NULL DEFAULT 0,
    issued_fp         VARCHAR(64)  NOT NULL DEFAULT '',
    live_fp           VARCHAR(64)  NOT NULL DEFAULT '',
    write_kind        VARCHAR(32)  NOT NULL DEFAULT '',
    summary           TEXT         NOT NULL,
    opened_at         DATETIME(3)  NOT NULL,
    exported_at       DATETIME(3)  NULL,
    detail            TEXT         NULL,
    fund_write_held   TINYINT(1)   NOT NULL DEFAULT 1,
    PRIMARY KEY (proof_id),
    KEY idx_otdp_lease (lease_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tick 漂移拒写证明仓';

CREATE TABLE IF NOT EXISTS oracle_tick_audit (
    audit_id    VARCHAR(64)  NOT NULL,
    lease_id    VARCHAR(64)  NULL,
    pair        VARCHAR(32)  NOT NULL DEFAULT '',
    action      VARCHAR(32)  NOT NULL,
    actor       VARCHAR(64)  NOT NULL,
    from_state  VARCHAR(16)  NOT NULL DEFAULT '',
    to_state    VARCHAR(16)  NOT NULL DEFAULT '',
    detail      VARCHAR(255) NOT NULL DEFAULT '',
    created_at  DATETIME(3)  NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_ota_lease (lease_id, created_at),
    KEY idx_ota_pair (pair, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='估值 Tick 租约审计';

CREATE TABLE IF NOT EXISTS oracle_tick_generations (
    pair        VARCHAR(32)  NOT NULL,
    generation  BIGINT       NOT NULL,
    price       VARCHAR(64)  NOT NULL,
    tick_at     DATETIME(3)  NOT NULL,
    updated_at  DATETIME(3)  NOT NULL,
    PRIMARY KEY (pair)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tick pair 世代';

CREATE TABLE IF NOT EXISTS oracle_tick_fund_writes (
    write_id    VARCHAR(64)  NOT NULL,
    lease_id    VARCHAR(64)  NOT NULL,
    write_kind  VARCHAR(32)  NOT NULL,
    write_ref   VARCHAR(64)  NOT NULL DEFAULT '',
    pair        VARCHAR(32)  NOT NULL,
    generation  BIGINT       NOT NULL,
    posted      TINYINT(1)   NOT NULL DEFAULT 1,
    actor       VARCHAR(64)  NOT NULL,
    posted_at   DATETIME(3)  NOT NULL,
    PRIMARY KEY (write_id),
    UNIQUE KEY uk_otfw_lease (lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='估值绑定资金写记录（一租约一写）';

CREATE TABLE IF NOT EXISTS oracle_tick_idem (
    idem_key   VARCHAR(128) NOT NULL,
    result_id  VARCHAR(64)  NOT NULL,
    PRIMARY KEY (idem_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tick 租约幂等';

-- +migrate Down

DROP TABLE IF EXISTS oracle_tick_idem;
DROP TABLE IF EXISTS oracle_tick_fund_writes;
DROP TABLE IF EXISTS oracle_tick_generations;
DROP TABLE IF EXISTS oracle_tick_audit;
DROP TABLE IF EXISTS oracle_tick_drift_proofs;
DROP TABLE IF EXISTS oracle_tick_leases;
