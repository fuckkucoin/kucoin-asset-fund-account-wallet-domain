-- 功能：闪兑冻额解冻互斥引擎持久化表——冻额效果令牌、双释案件、终态腿绑定、冻额世代。
-- 边界：四张表均追加写，state/status 字段由业务层 FSM 控制，禁止直接 UPDATE 绕过状态机。
-- 不变量：
--   sw_freeze_leases.status 合法值：open / settled / thawed / blocked（后三者为终态）。
--   sw_thaw_cases.status 合法值：open / proven / closed（终态）。
--   sw_settle_binds.order_id 唯一约束——同一订单只允许一条终态腿绑定记录。
--   sw_freeze_generations.generation 单调递增，禁止减少或置零。
-- 风险点：
--   sw_freeze_leases 不存储 token 字段（token 仅在内存传递，防止泄漏到审计日志）。
--   sw_thaw_cases.frozen_snapshot / available_snapshot 落库后禁止 UPDATE（事故现场证据）。
--   swap_orders 新增 freeze_lease_id / thaw_proof_id 列，兼容旧行（NULL 允许）。
--
-- Author: TianpengLan
-- 作者：Kinsley
-- Date: 2021-11-08

BEGIN;

-- ──────────────────────────────────────────────
-- 冻额效果令牌表（每次 Execute 冻额动作产出一条记录）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sw_freeze_leases (
    id              VARCHAR(64)     NOT NULL PRIMARY KEY,
    order_id        VARCHAR(64)     NOT NULL,
    user_id         VARCHAR(64)     NOT NULL,
    asset           VARCHAR(32)     NOT NULL,
    frozen_amount   DECIMAL(36,18)  NOT NULL DEFAULT 0,
    generation      BIGINT          NOT NULL DEFAULT 1,
    status          VARCHAR(16)     NOT NULL DEFAULT 'open',
    expires_at      TIMESTAMPTZ,
    winner_leg      VARCHAR(16),
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_sfl_status    CHECK (status IN ('open','settled','thawed','blocked')),
    CONSTRAINT chk_sfl_leg       CHECK (winner_leg IS NULL OR winner_leg IN ('settle','fail_unfreeze')),
    CONSTRAINT chk_sfl_amount    CHECK (frozen_amount >= 0),
    CONSTRAINT chk_sfl_generation CHECK (generation >= 1)
);

CREATE INDEX IF NOT EXISTS idx_sfl_order_status ON sw_freeze_leases (order_id, status);
CREATE INDEX IF NOT EXISTS idx_sfl_user         ON sw_freeze_leases (user_id);
CREATE INDEX IF NOT EXISTS idx_sfl_expires      ON sw_freeze_leases (expires_at) WHERE status = 'open';

-- ──────────────────────────────────────────────
-- 冻额世代表（每个订单一行，generation 单调递增）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sw_freeze_generations (
    order_id        VARCHAR(64)     NOT NULL PRIMARY KEY,
    user_id         VARCHAR(64)     NOT NULL,
    generation      BIGINT          NOT NULL DEFAULT 0,
    active_lease_id VARCHAR(64),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_sfg_generation CHECK (generation >= 0)
);

CREATE INDEX IF NOT EXISTS idx_sfg_user ON sw_freeze_generations (user_id);

-- ──────────────────────────────────────────────
-- 终态腿互斥绑定表（同一 order_id 仅允许一条记录）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sw_settle_binds (
    id              VARCHAR(64)     NOT NULL PRIMARY KEY,
    order_id        VARCHAR(64)     NOT NULL UNIQUE,
    lease_id        VARCHAR(64)     NOT NULL,
    leg             VARCHAR(16)     NOT NULL,
    actor           VARCHAR(128),
    bound_at        TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_ssb_leg CHECK (leg IN ('settle','fail_unfreeze'))
);

CREATE INDEX IF NOT EXISTS idx_ssb_lease ON sw_settle_binds (lease_id);

-- ──────────────────────────────────────────────
-- 双释案件表（双释/孤儿 Frozen 事故证明仓）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sw_thaw_cases (
    id                  VARCHAR(64)     NOT NULL PRIMARY KEY,
    order_id            VARCHAR(64)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL,
    lease_id            VARCHAR(64),
    asset               VARCHAR(32)     NOT NULL,
    kind                VARCHAR(32)     NOT NULL,
    status              VARCHAR(16)     NOT NULL DEFAULT 'open',
    detected_amount     DECIMAL(36,18)  NOT NULL DEFAULT 0,
    -- frozen_snapshot / available_snapshot 落库后禁止 UPDATE，作为事故现场证据。
    frozen_snapshot     DECIMAL(36,18)  NOT NULL DEFAULT 0,
    available_snapshot  DECIMAL(36,18)  NOT NULL DEFAULT 0,
    proof_payload       TEXT,
    approver            VARCHAR(128),
    detected_at         TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    proven_at           TIMESTAMPTZ,
    closed_at           TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_stc_status CHECK (status IN ('open','proven','closed')),
    CONSTRAINT chk_stc_kind   CHECK (kind IN ('double_release','orphan_frozen','concurrent_thaw')),
    CONSTRAINT chk_stc_amount CHECK (detected_amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_stc_order      ON sw_thaw_cases (order_id);
CREATE INDEX IF NOT EXISTS idx_stc_user_status ON sw_thaw_cases (user_id, status);
CREATE INDEX IF NOT EXISTS idx_stc_status     ON sw_thaw_cases (status, detected_at DESC) WHERE status IN ('open','proven');

-- ──────────────────────────────────────────────
-- swap_orders 扩展列（兼容旧行，NULL 允许）
-- ──────────────────────────────────────────────
ALTER TABLE swap_orders ADD COLUMN IF NOT EXISTS freeze_lease_id VARCHAR(64)  DEFAULT NULL;
ALTER TABLE swap_orders ADD COLUMN IF NOT EXISTS thaw_proof_id   VARCHAR(64)  DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_so_freeze_lease ON swap_orders (freeze_lease_id) WHERE freeze_lease_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_so_thaw_proof   ON swap_orders (thaw_proof_id)   WHERE thaw_proof_id IS NOT NULL;

COMMIT;
// temporary guard removed after the production check
