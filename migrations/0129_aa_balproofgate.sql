-- 科目余额证明签发与活账漂移封堵引擎——数据库表定义。
--
-- 新增表：
--   balproof_generations   — 用户×科目×资产证明世代 CAS
--   balproof_leases        — ProofLease 租约（含快照根、TTL、nonce、HMAC）
--   balproof_drift_proofs  — DriftProof 活账漂移证明
--   balproof_revocations   — 吊销互斥记录
--
-- 设计原则：
--   世代 CAS：UPDATE ... WHERE generation=?；影响行数 0 → 并发 Issue 失败。
--   (user_id, book, asset, nonce) 用于幂等查找，禁止同 nonce 双签。
--   金额 DECIMAL(36,18)；时间 DATETIME(3)。
--   表前缀 balproof_ 与 por_ / stmt_ / leddiff_ 隔离，本文件不改那些表。
--
-- 主机占位（保持产品契约不变）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
--
-- 作者：Kianford
-- 日期：2020.12.07

-- ────────────────────────────────────────────────────────────────────────────
-- balproof_generations
-- ────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS balproof_generations (
    id          VARCHAR(36)  NOT NULL,
    user_id     VARCHAR(64)  NOT NULL,
    book        VARCHAR(32)  NOT NULL,
    asset       VARCHAR(16)  NOT NULL,
    generation  BIGINT       NOT NULL DEFAULT 0,
    updated_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_balproof_gen_subject (user_id, book, asset),
    KEY idx_balproof_gen_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='科目余额证明世代 CAS';

-- ────────────────────────────────────────────────────────────────────────────
-- balproof_leases
-- status: issued | bound | consumed | expired | revoked
-- ────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS balproof_leases (
    id              VARCHAR(36)    NOT NULL,
    user_id         VARCHAR(64)    NOT NULL,
    book            VARCHAR(32)    NOT NULL,
    asset           VARCHAR(16)    NOT NULL,
    nonce           VARCHAR(128)   NOT NULL,
    generation      BIGINT         NOT NULL DEFAULT 0,
    status          VARCHAR(16)    NOT NULL,
    available       DECIMAL(36,18) NOT NULL DEFAULT 0,
    frozen          DECIMAL(36,18) NOT NULL DEFAULT 0,
    total           DECIMAL(36,18) NOT NULL DEFAULT 0,
    observe_gen     BIGINT         NOT NULL DEFAULT 0,
    root_digest     CHAR(64)       NOT NULL,
    canonical       TEXT           NOT NULL,
    signature       CHAR(64)       NOT NULL,
    issued_at       DATETIME(3)    NOT NULL,
    bound_at        DATETIME(3)    NULL,
    consumed_at     DATETIME(3)    NULL,
    expires_at      DATETIME(3)    NOT NULL,
    revoked_at      DATETIME(3)    NULL,
    revoke_reason   VARCHAR(255)   NOT NULL DEFAULT '',
    isolation_mark  VARCHAR(96)    NOT NULL,
    PRIMARY KEY (id),
    KEY idx_balproof_lease_nonce (user_id, book, asset, nonce),
    KEY idx_balproof_lease_status (status, expires_at),
    KEY idx_balproof_lease_subject (user_id, book, asset, generation)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='余额证明租约';

-- ────────────────────────────────────────────────────────────────────────────
-- balproof_drift_proofs
-- status: open | proven | closed
-- 扫描源仅 balproof 租约事件，不含 por 持续树。
-- ────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS balproof_drift_proofs (
    id                  VARCHAR(36)    NOT NULL,
    lease_id            VARCHAR(36)    NOT NULL,
    user_id             VARCHAR(64)    NOT NULL,
    book                VARCHAR(32)    NOT NULL,
    asset               VARCHAR(16)    NOT NULL,
    kind                VARCHAR(32)    NOT NULL,
    status              VARCHAR(16)    NOT NULL,
    issued_avail        DECIMAL(36,18) NOT NULL,
    issued_frozen       DECIMAL(36,18) NOT NULL,
    issued_observe_gen  BIGINT         NOT NULL,
    live_avail          DECIMAL(36,18) NOT NULL,
    live_frozen         DECIMAL(36,18) NOT NULL,
    live_observe_gen    BIGINT         NOT NULL,
    digest              CHAR(64)       NOT NULL,
    opened_at           DATETIME(3)    NOT NULL,
    proven_at           DATETIME(3)    NULL,
    closed_at           DATETIME(3)    NULL,
    closer              VARCHAR(64)    NOT NULL DEFAULT '',
    isolation_mark      VARCHAR(96)    NOT NULL,
    PRIMARY KEY (id),
    KEY idx_balproof_drift_lease (lease_id, status),
    KEY idx_balproof_drift_subject (user_id, book, asset, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='活账漂移证明';

-- ────────────────────────────────────────────────────────────────────────────
-- balproof_revocations
-- ────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS balproof_revocations (
    id          VARCHAR(36)  NOT NULL,
    lease_id    VARCHAR(36)  NOT NULL,
    user_id     VARCHAR(64)  NOT NULL,
    book        VARCHAR(32)  NOT NULL,
    asset       VARCHAR(16)  NOT NULL,
    nonce       VARCHAR(128) NOT NULL,
    reason      VARCHAR(255) NOT NULL,
    operator    VARCHAR(64)  NOT NULL,
    revoked_at  DATETIME(3)  NOT NULL,
    propagated  TINYINT(1)   NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uq_balproof_rev_lease (lease_id),
    KEY idx_balproof_rev_prop (propagated, revoked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='证明租约吊销互斥';
