-- 功能：出金分片部分成交剩余释放与成交确认竞态收口持久化表。
-- 边界：五张表均追加写；status 字段由业务层 FSM 控制，禁止直接 UPDATE 绕过状态机。
-- 不变量：
--   wdpartial_rem_leases.generation    单调递增，禁止减少或置零。
--   wdpartial_rem_leases.status        合法值：open / releasing / blocked / done / revoked（后两者为终态）。
--   wdpartial_conserve_cases.status    合法值：open / proven / closed（终态）。
--   wdpartial_conserve_cases.conserve_snapshot 落库后禁止 UPDATE（事故现场证据）。
--   wdpartial_guard_gens.generation    单调递增，禁止减少或置零。
--   wdpartial_guard_gens.status        合法值：active / invalidated / consumed（后两者为终态）。
-- 风险点：
--   wdpartial_rem_leases 不存储 fill_mutex_token 明文（令牌仅在内存传递，防止泄漏）。
--   wdpartial_conserve_cases.conserve_snapshot / fail_reason 须通过 BumpConserveCase CAS 写入；
--   直接 UPDATE 破坏守恒现场不变量。
--   wdpartial_guard_gens open 状态期间，关联 skip 腿 Hold 不得释放（业务层保证，非 DB 约束）。
--
-- Author: TianpengLan
-- 作者：Kinsley
-- Date: 2021-03-18

BEGIN;

-- ──────────────────────────────────────────────
-- Hold 冻结记录表
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS wdpartial_holds (
    id               VARCHAR(64)     NOT NULL PRIMARY KEY,
    plan_id          VARCHAR(64)     NOT NULL,
    user_id          VARCHAR(64)     NOT NULL,
    asset            VARCHAR(32)     NOT NULL,
    amount           DECIMAL(36,18)  NOT NULL DEFAULT 0,
    seq              INT             NOT NULL DEFAULT 0,
    status           VARCHAR(16)     NOT NULL DEFAULT 'open',
    guard_gen_id     VARCHAR(64)     DEFAULT NULL,
    released_at      DATETIME(3)     DEFAULT NULL,
    consumed_at      DATETIME(3)     DEFAULT NULL,
    created_at       DATETIME(3)     NOT NULL,
    updated_at       DATETIME(3)     NOT NULL,

    CONSTRAINT chk_wph_status CHECK (status IN ('open','released','consumed','void')),
    CONSTRAINT chk_wph_amount CHECK (amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_wph_plan_status   ON wdpartial_holds (plan_id, status);
CREATE INDEX IF NOT EXISTS idx_wph_guard_gen      ON wdpartial_holds (guard_gen_id) WHERE guard_gen_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_wph_user_plan      ON wdpartial_holds (user_id, plan_id);

-- ──────────────────────────────────────────────
-- Fill 部分成交记录表
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS wdpartial_fills (
    id               VARCHAR(64)     NOT NULL PRIMARY KEY,
    plan_id          VARCHAR(64)     NOT NULL,
    hold_id          VARCHAR(64)     NOT NULL,
    user_id          VARCHAR(64)     NOT NULL,
    asset            VARCHAR(32)     NOT NULL,
    amount           DECIMAL(36,18)  NOT NULL DEFAULT 0,
    seq              INT             NOT NULL DEFAULT 0,
    status           VARCHAR(16)     NOT NULL DEFAULT 'pending',
    mutex_token_id   VARCHAR(64)     DEFAULT NULL,
    confirmed_at     DATETIME(3)     DEFAULT NULL,
    rejected_at      DATETIME(3)     DEFAULT NULL,
    created_at       DATETIME(3)     NOT NULL,
    updated_at       DATETIME(3)     NOT NULL,

    CONSTRAINT chk_wpf_status CHECK (status IN ('pending','confirmed','rejected')),
    CONSTRAINT chk_wpf_amount CHECK (amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_wpf_plan_status    ON wdpartial_fills (plan_id, status);
CREATE INDEX IF NOT EXISTS idx_wpf_hold           ON wdpartial_fills (hold_id);
CREATE INDEX IF NOT EXISTS idx_wpf_plan_confirmed ON wdpartial_fills (plan_id) WHERE status = 'confirmed';

-- ──────────────────────────────────────────────
-- 剩余释放租约表（rem_release_gen 互斥世代）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS wdpartial_rem_leases (
    id                  VARCHAR(64)     NOT NULL PRIMARY KEY,
    plan_id             VARCHAR(64)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL,
    -- generation：rem_release_gen，单调递增，禁止减少或置零。
    generation          BIGINT          NOT NULL DEFAULT 1,
    status              VARCHAR(16)     NOT NULL DEFAULT 'open',
    -- blocked_by_fill_id：FillMutexToken 持有期间记录阻塞来源 fill_id。
    blocked_by_fill_id  VARCHAR(64)     DEFAULT NULL,
    expires_at          DATETIME(3)     NOT NULL,
    releasing_at        DATETIME(3)     DEFAULT NULL,
    done_at             DATETIME(3)     DEFAULT NULL,
    revoked_at          DATETIME(3)     DEFAULT NULL,
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,

    CONSTRAINT chk_wprl_status      CHECK (status IN ('open','releasing','blocked','done','revoked')),
    CONSTRAINT chk_wprl_generation  CHECK (generation >= 1)
);

CREATE INDEX IF NOT EXISTS idx_wprl_plan_status   ON wdpartial_rem_leases (plan_id, status);
CREATE INDEX IF NOT EXISTS idx_wprl_expires        ON wdpartial_rem_leases (expires_at) WHERE status NOT IN ('done','revoked');
CREATE INDEX IF NOT EXISTS idx_wprl_plan_open      ON wdpartial_rem_leases (plan_id)    WHERE status = 'open';

-- ──────────────────────────────────────────────
-- 守恒案件表（PlanConservation 失败升级仓）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS wdpartial_conserve_cases (
    id                  VARCHAR(64)     NOT NULL PRIMARY KEY,
    plan_id             VARCHAR(64)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL DEFAULT '',
    status              VARCHAR(16)     NOT NULL DEFAULT 'open',
    -- conserve_snapshot：守恒失败时的计划状态快照，落库后禁止 UPDATE（事故现场证据）。
    conserve_snapshot   TEXT            NOT NULL DEFAULT '',
    fail_reason         TEXT            NOT NULL DEFAULT '',
    proof_payload       TEXT            DEFAULT NULL,
    approver            VARCHAR(128)    DEFAULT NULL,
    approver2           VARCHAR(128)    DEFAULT NULL,
    detected_at         DATETIME(3)     NOT NULL,
    proven_at           DATETIME(3)     DEFAULT NULL,
    closed_at           DATETIME(3)     DEFAULT NULL,
    updated_at          DATETIME(3)     NOT NULL,

    CONSTRAINT chk_wpcc_status  CHECK (status IN ('open','proven','closed'))
);

CREATE INDEX IF NOT EXISTS idx_wpcc_plan_status   ON wdpartial_conserve_cases (plan_id, status);
CREATE INDEX IF NOT EXISTS idx_wpcc_status_det    ON wdpartial_conserve_cases (status, detected_at DESC) WHERE status IN ('open','proven');
CREATE INDEX IF NOT EXISTS idx_wpcc_plan_open     ON wdpartial_conserve_cases (plan_id) WHERE status = 'open';

-- ──────────────────────────────────────────────
-- 广播守卫世代表（skipBroadcasting 腿 live-hash 守卫）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS wdpartial_guard_gens (
    id                  VARCHAR(64)     NOT NULL PRIMARY KEY,
    plan_id             VARCHAR(64)     NOT NULL,
    -- live_hash：skip 腿绑定的广播 live-hash 值（内容寻址，不使用哈希明文作为密钥）。
    live_hash           VARCHAR(128)    NOT NULL DEFAULT '',
    generation          BIGINT          NOT NULL DEFAULT 1,
    status              VARCHAR(16)     NOT NULL DEFAULT 'active',
    invalidated_reason  VARCHAR(256)    DEFAULT NULL,
    invalidated_at      DATETIME(3)     DEFAULT NULL,
    consumed_at         DATETIME(3)     DEFAULT NULL,
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,

    CONSTRAINT chk_wpgg_status      CHECK (status IN ('active','invalidated','consumed')),
    CONSTRAINT chk_wpgg_generation  CHECK (generation >= 1)
);

CREATE INDEX IF NOT EXISTS idx_wpgg_plan_status   ON wdpartial_guard_gens (plan_id, status);
CREATE INDEX IF NOT EXISTS idx_wpgg_plan_active   ON wdpartial_guard_gens (plan_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_wpgg_updated        ON wdpartial_guard_gens (updated_at);

COMMIT;
