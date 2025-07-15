-- 功能：围栏旁路收口引擎持久化表——围栏租约、旁路案件、主子守恒证明、围栏世代。
-- 边界：四张表均仅追加写，status/state 字段由业务层 FSM 控制，禁止直接 UPDATE 绕过状态机。
-- 不变量：
--   broker_fence_leases.state 合法值：open / active / lifting / sealed（终态）。
--   broker_bypass_cases.state 合法值：open / proven / closed（终态）。
--   broker_fence_generations.generation 单调递增，禁止减少或置零。
--   broker_conserve_proofs 一个 lease 可有多条（多次抽样），以 sampled_at 排序取最新。
-- 风险点：
--   broker_bypass_cases.hanging_amount 在 state=closed 前不得清零（资金安全不变量）。
--   broker_fence_leases.generation 与 broker_fence_generations.generation 须保持一致。
--
-- Author: TianpengLan
-- 作者：TianpengLan
-- Date: 2021-11-15

BEGIN;

-- 围栏世代表（每个子账一行，generation 单调递增）。
CREATE TABLE IF NOT EXISTS broker_fence_generations (
    sub_id          VARCHAR(64)     NOT NULL PRIMARY KEY,
    generation      BIGINT          NOT NULL DEFAULT 0,
    fenced          BOOLEAN         NOT NULL DEFAULT FALSE,
    fenced_since    TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_generation_nonneg CHECK (generation >= 0)
);

-- 围栏租约表（每次 Fence/Unfence 产出一条记录）。
CREATE TABLE IF NOT EXISTS broker_fence_leases (
    id              VARCHAR(64)     NOT NULL PRIMARY KEY,
    sub_id          VARCHAR(64)     NOT NULL,
    master_id       VARCHAR(64)     NOT NULL,
    asset           VARCHAR(32)     NOT NULL,
    generation      BIGINT          NOT NULL,
    state           VARCHAR(16)     NOT NULL DEFAULT 'open',
    actor           VARCHAR(128)    NOT NULL,
    proof_id        VARCHAR(64),
    reason          TEXT,
    opened_at       TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    active_at       TIMESTAMPTZ,
    lifting_at      TIMESTAMPTZ,
    sealed_at       TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_lease_state CHECK (state IN ('open', 'active', 'lifting', 'sealed'))
);

CREATE INDEX IF NOT EXISTS idx_bfl_sub_state ON broker_fence_leases (sub_id, state);
CREATE INDEX IF NOT EXISTS idx_bfl_master     ON broker_fence_leases (master_id);

-- 旁路案件表（围栏窗口内每次旁路写操作产出一条记录）。
CREATE TABLE IF NOT EXISTS broker_bypass_cases (
    id              VARCHAR(64)     NOT NULL PRIMARY KEY,
    lease_id        VARCHAR(64)     NOT NULL REFERENCES broker_fence_leases(id),
    sub_id          VARCHAR(64)     NOT NULL,
    master_id       VARCHAR(64)     NOT NULL,
    asset           VARCHAR(32)     NOT NULL,
    amount          DECIMAL(36,18)  NOT NULL DEFAULT 0,
    kind            VARCHAR(32)     NOT NULL,
    state           VARCHAR(16)     NOT NULL DEFAULT 'open',
    hanging_amount  DECIMAL(36,18)  NOT NULL DEFAULT 0,
    approver_one    VARCHAR(128),
    approver_two    VARCHAR(128),
    proven_at       TIMESTAMPTZ,
    closed_at       TIMESTAMPTZ,
    detected_at     TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_bypass_state CHECK (state IN ('open', 'proven', 'closed')),
    CONSTRAINT chk_bypass_kind  CHECK (kind IN ('to_sub', 'to_master', 'credit_master', 'fill')),
    CONSTRAINT chk_hanging_nonneg CHECK (hanging_amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_bbc_lease     ON broker_bypass_cases (lease_id);
CREATE INDEX IF NOT EXISTS idx_bbc_sub_state ON broker_bypass_cases (sub_id, state);

-- 主子守恒证明表（每次抽样产出一条记录，同一 lease 可有多条）。
CREATE TABLE IF NOT EXISTS broker_conserve_proofs (
    id              VARCHAR(64)     NOT NULL PRIMARY KEY,
    lease_id        VARCHAR(64)     NOT NULL REFERENCES broker_fence_leases(id),
    sub_id          VARCHAR(64)     NOT NULL,
    master_id       VARCHAR(64)     NOT NULL,
    asset           VARCHAR(32)     NOT NULL,
    before_total    DECIMAL(36,18)  NOT NULL DEFAULT 0,
    after_total     DECIMAL(36,18),
    delta           DECIMAL(36,18)  NOT NULL DEFAULT 0,
    is_drifted      BOOLEAN         NOT NULL DEFAULT FALSE,
    sampled_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bcp_lease ON broker_conserve_proofs (lease_id, sampled_at DESC);

COMMIT;
