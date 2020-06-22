-- 功能：毒价事件持久化表与写门令牌审计表。
-- 边界：poison_event 为追加写，不得更新/删除（审计不可篡改）。
--       write_gate_token 记录每次写门通过/拒绝的令牌快照，供离线审计。
-- 不变量：
--   - id 自增主键；pair 已规范化（大写 ASSET/QUOTE）。
--   - event_type: spike / invert / cross_dev / stale_mask。
--   - gate_result: allowed / blocked_token / blocked_probe / blocked_scope。
--
-- 作者：Cason
-- 日期：2020-12-27

BEGIN;

-- 毒价事件仓库（持久化，只追加）。
CREATE TABLE IF NOT EXISTS aa_price_poison_event (
    id              BIGSERIAL        PRIMARY KEY,
    pair            VARCHAR(32)      NOT NULL,
    tick_price      VARCHAR(64)      NOT NULL,
    tick_at         TIMESTAMPTZ      NOT NULL,
    event_type      VARCHAR(32)      NOT NULL, -- spike / invert / cross_dev / stale_mask
    rule            VARCHAR(64)      NOT NULL,
    observed        VARCHAR(128)     NOT NULL,
    threshold_val   VARCHAR(128)     NOT NULL,
    extra           TEXT             NOT NULL DEFAULT '',
    generation_rev  BIGINT           NOT NULL DEFAULT 0, -- 被撤销的代次
    detected_at     TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_event_type CHECK (event_type IN ('spike','invert','cross_dev','stale_mask'))
);

CREATE INDEX IF NOT EXISTS idx_aa_price_poison_event_pair
    ON aa_price_poison_event (pair, detected_at DESC);

-- 写门令牌审计快照（持久化，只追加）。
CREATE TABLE IF NOT EXISTS aa_write_gate_token (
    id              BIGSERIAL        PRIMARY KEY,
    pair            VARCHAR(32)      NOT NULL,
    generation      BIGINT           NOT NULL,
    tick_digest     VARCHAR(128)     NOT NULL, -- SHA-256 hex
    write_scope     VARCHAR(64)      NOT NULL, -- credit / occupancy / liquidation_trigger
    gate_result     VARCHAR(32)      NOT NULL, -- allowed / blocked_token / blocked_probe / blocked_scope
    block_reason    TEXT             NOT NULL DEFAULT '',
    proof_digest    VARCHAR(128)     NOT NULL DEFAULT '', -- PoisonProof.Digest (hex)
    issued_at       TIMESTAMPTZ      NOT NULL,
    checked_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_scope    CHECK (write_scope IN ('credit','occupancy','liquidation_trigger','*')),
    CONSTRAINT chk_result   CHECK (gate_result IN ('allowed','blocked_token','blocked_probe','blocked_scope','blocked_unknown'))
);

CREATE INDEX IF NOT EXISTS idx_aa_write_gate_token_pair
    ON aa_write_gate_token (pair, checked_at DESC);

CREATE INDEX IF NOT EXISTS idx_aa_write_gate_token_scope
    ON aa_write_gate_token (write_scope, gate_result, checked_at DESC);

COMMIT;
