-- 功能：交易密码世代引擎持久化表——密码世代、核验令牌、漂移案件。
-- 边界：三张表均追加写，status 字段由业务层 FSM 控制，禁止直接 UPDATE 绕过状态机。
-- 不变量：
--   tradepw_generations.generation 单调递增，禁止减少或置零。
--   tradepw_verify_leases.status 合法值：open / consumed / revoked（后两者为终态）。
--   tradepw_drift_cases.status 合法值：open / proven / closed（终态）。
--   tradepw_drift_cases.verify_snapshot / gen_snapshot 落库后禁止 UPDATE（事故现场证据）。
-- 风险点：
--   tradepw_verify_leases 不存储 token 明文（token 仅在内存传递，防止泄漏到审计日志）。
--   tradepw_generations.generation 须通过 BumpGeneration CAS 写入；直接 UPDATE 破坏不变量。
--   tradepw_drift_cases open 状态期间，新核验令牌颁发须被阻断（业务层保证，非 DB 约束）。
--
-- Author: TianpengLan
-- 作者：TianpengLan
-- Date: 2021-03-18

BEGIN;

-- ──────────────────────────────────────────────
-- 密码世代表（每个用户一行，generation 单调递增）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS tradepw_generations (
    user_id          VARCHAR(64)     NOT NULL PRIMARY KEY,
    generation       BIGINT          NOT NULL DEFAULT 0,
    status           VARCHAR(16)     NOT NULL DEFAULT 'active',
    cool_bound       TINYINT(1)      NOT NULL DEFAULT 0,
    active_lease_id  VARCHAR(64)     DEFAULT NULL,
    created_at       DATETIME(3)     NOT NULL,
    updated_at       DATETIME(3)     NOT NULL,

    CONSTRAINT chk_tpg_generation CHECK (generation >= 0),
    CONSTRAINT chk_tpg_status     CHECK (status IN ('active', 'superseded'))
);

CREATE INDEX IF NOT EXISTS idx_tpg_cool     ON tradepw_generations (cool_bound, generation) WHERE cool_bound = 0;
CREATE INDEX IF NOT EXISTS idx_tpg_updated  ON tradepw_generations (updated_at);

-- ──────────────────────────────────────────────
-- 核验令牌表（每次 RequireForWithdraw 通过后插入一行）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS tradepw_verify_leases (
    id               VARCHAR(64)     NOT NULL PRIMARY KEY,
    user_id          VARCHAR(64)     NOT NULL,
    pw_gen           BIGINT          NOT NULL DEFAULT 0,
    -- token_hash：token 仅存 sha256 散列，防止明文泄漏到数据库层审计日志。
    token_hash       VARCHAR(64)     NOT NULL DEFAULT '',
    status           VARCHAR(16)     NOT NULL DEFAULT 'open',
    expires_at       DATETIME(3)     DEFAULT NULL,
    consumed_at      DATETIME(3)     DEFAULT NULL,
    revoked_at       DATETIME(3)     DEFAULT NULL,
    created_at       DATETIME(3)     NOT NULL,
    updated_at       DATETIME(3)     NOT NULL,

    CONSTRAINT chk_tpvl_status     CHECK (status IN ('open','consumed','revoked')),
    CONSTRAINT chk_tpvl_gen        CHECK (pw_gen >= 0)
);

CREATE INDEX IF NOT EXISTS idx_tpvl_user_status  ON tradepw_verify_leases (user_id, status);
CREATE INDEX IF NOT EXISTS idx_tpvl_expires       ON tradepw_verify_leases (expires_at) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_tpvl_user_open     ON tradepw_verify_leases (user_id)    WHERE status = 'open';

-- ──────────────────────────────────────────────
-- 漂移案件表（旧 gen 令牌核验漂移事故证明仓）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS tradepw_drift_cases (
    id               VARCHAR(64)     NOT NULL PRIMARY KEY,
    user_id          VARCHAR(64)     NOT NULL,
    lease_id         VARCHAR(64)     NOT NULL DEFAULT '',
    verify_gen       BIGINT          NOT NULL DEFAULT 0,
    current_gen      BIGINT          NOT NULL DEFAULT 0,
    status           VARCHAR(16)     NOT NULL DEFAULT 'open',
    -- verify_snapshot / gen_snapshot 落库后禁止 UPDATE，作为事故现场证据。
    verify_snapshot  TEXT            NOT NULL DEFAULT '',
    gen_snapshot     TEXT            NOT NULL DEFAULT '',
    proof_payload    TEXT            DEFAULT NULL,
    approver         VARCHAR(128)    DEFAULT NULL,
    approver2        VARCHAR(128)    DEFAULT NULL,
    detected_at      DATETIME(3)     NOT NULL,
    proven_at        DATETIME(3)     DEFAULT NULL,
    closed_at        DATETIME(3)     DEFAULT NULL,
    updated_at       DATETIME(3)     NOT NULL,

    CONSTRAINT chk_tpdc_status     CHECK (status IN ('open','proven','closed')),
    CONSTRAINT chk_tpdc_verify_gen CHECK (verify_gen >= 0),
    CONSTRAINT chk_tpdc_cur_gen    CHECK (current_gen >= 0)
);

CREATE INDEX IF NOT EXISTS idx_tpdc_user_status  ON tradepw_drift_cases (user_id, status);
CREATE INDEX IF NOT EXISTS idx_tpdc_status_det   ON tradepw_drift_cases (status, detected_at DESC) WHERE status IN ('open','proven');
CREATE INDEX IF NOT EXISTS idx_tpdc_lease        ON tradepw_drift_cases (lease_id);

COMMIT;
