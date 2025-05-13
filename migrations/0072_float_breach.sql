-- 浮存与 gas 仓穿仓证明持久化表。
-- 记录每次补仓计划对应的穿仓证明摘要，供离线审计和竞态封堵追溯。
--
-- 作者：Cason
-- 日期：2020.12.21

-- float_breach_proof 穿仓证明流水。
-- 不变量：(chain, native, issued_at) 组合唯一，同时刻同链至多一条证明。
CREATE TABLE IF NOT EXISTS float_breach_proof (
    id            BIGSERIAL PRIMARY KEY,
    chain         VARCHAR(32)  NOT NULL,
    native        VARCHAR(32)  NOT NULL,
    water_level   NUMERIC(36,18) NOT NULL,   -- 补仓前 Avail 快照
    refill_amount NUMERIC(36,18) NOT NULL,   -- 本次计划补仓金额
    window_ns     BIGINT       NOT NULL,     -- 评估窗口（纳秒）
    breach_level  SMALLINT     NOT NULL,     -- 0=OK 1=Warn 2=Crit 3=Halt
    digest        CHAR(64)     NOT NULL,     -- SHA-256 十六进制摘要
    issued_at     TIMESTAMPTZ  NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_float_breach_proof UNIQUE (chain, native, issued_at)
);

CREATE INDEX IF NOT EXISTS idx_float_breach_proof_chain_level
    ON float_breach_proof (chain, breach_level, issued_at DESC);

-- float_refill_lock_log 补仓互斥锁冲突日志。
-- 记录每次 ErrRefillConflict 事件，用于竞态封堵审计。
CREATE TABLE IF NOT EXISTS float_refill_lock_log (
    id           BIGSERIAL PRIMARY KEY,
    chain        VARCHAR(32) NOT NULL,
    generation   BIGINT      NOT NULL,   -- 冲突时活跃令牌的代次
    conflict_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_float_refill_lock_log_chain
    ON float_refill_lock_log (chain, conflict_at DESC);
