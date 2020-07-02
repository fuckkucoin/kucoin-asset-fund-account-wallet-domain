-- 放贷挂单占用与成交取消双释竞态收口引擎表族。
-- 含：lend_gens（放贷世代令牌）、lend_occupies（占用租约）、lend_proofs（双释/半截证明）。
-- 不含：loanbook 逾期冻结、marginhook LTV 强平扣押、P2P 保证金弃单相关表。
--
-- 作者：Eco
-- 日期：2025.12.01

-- ── lend_gens ─────────────────────────────────────────────────────────────────
-- 放贷世代令牌：Place 成功后签发；Settle 或 Cancel 消费（consumed→1）。
-- 同 order_id 至多一个 active gen（consumed=0）；已消费不可逆。
-- INSERT IGNORE 保证幂等写入；不提供 DELETE。
CREATE TABLE IF NOT EXISTS lend_gens (
    gen_id              VARCHAR(36)     NOT NULL,
    order_id            VARCHAR(64)     NOT NULL,       -- 放贷挂单外部 ID（全局唯一）
    user_id             VARCHAR(64)     NOT NULL,       -- 下单用户 ID
    asset               VARCHAR(16)     NOT NULL,       -- 资产代码（大写）
    amount              DECIMAL(36,18)  NOT NULL,       -- 挂单金额（精确到 18 位）
    ledger_idem_key     VARCHAR(128)    NOT NULL DEFAULT '', -- 账本借记幂等键（"lend-ord:" + order_id）
    consumed            TINYINT(1)      NOT NULL DEFAULT 0,  -- 0=未消费，1=已消费（不可逆）
    consumed_by         VARCHAR(16)     NOT NULL DEFAULT '', -- settle | cancel | residue
    consumed_at         DATETIME(3)     NULL,                -- 消费时间（UTC）
    issued_at           DATETIME(3)     NOT NULL,            -- 签发时间（UTC）
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (gen_id),
    UNIQUE KEY uq_lg_order_id (order_id),               -- 同 order 至多一个 gen
    KEY idx_lg_user_id (user_id),
    KEY idx_lg_consumed_issued (consumed, issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='放贷世代令牌（lend_gen）';

-- ── lend_occupies ─────────────────────────────────────────────────────────────
    user_id             VARCHAR(64)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    amount              DECIMAL(36,18)  NOT NULL,
    state               VARCHAR(20)     NOT NULL DEFAULT 'placed',
        -- placed | settled | cancelled | residue_closed
    ledger_idem_key     VARCHAR(128)    NOT NULL DEFAULT '',
    hold_ref            VARCHAR(64)     NOT NULL DEFAULT '', -- holdcat 标签引用（= order_id）
    placed_at           DATETIME(3)     NOT NULL,           -- 挂单时间（UTC）
    settled_at          DATETIME(3)     NULL,               -- 成交关闭时间（UTC）
    cancelled_at        DATETIME(3)     NULL,               -- 取消关闭时间（UTC）
    residue_closed_at   DATETIME(3)     NULL,               -- 半截补偿关闭时间（UTC）
    residue_proof_id    VARCHAR(36)     NOT NULL DEFAULT '', -- 关联 LendResidueProof.ProofID
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (lease_id),
    UNIQUE KEY uq_lo_order_id (order_id),
    KEY idx_lo_gen_id (gen_id),
    KEY idx_lo_user_id (user_id),
    KEY idx_lo_state_placed (state, placed_at),        -- 半截扫描核心索引
    KEY idx_lo_asset_state (asset, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='放贷挂单占用租约';

-- ── lend_proofs ───────────────────────────────────────────────────────────────
-- 统一证明表：dup（重复释放）与 residue（半截补偿）共用同一张表。
-- proof_type 区分类型；通用字段共用，类型专用字段 NULL 表示不适用。
-- 只追加，不修改；同 gen_id+proof_type 至多一条（uq_lp_gen_type）。
CREATE TABLE IF NOT EXISTS lend_proofs (
    proof_id            VARCHAR(36)     NOT NULL,
    proof_type          VARCHAR(16)     NOT NULL,       -- dup | residue
    gen_id              VARCHAR(36)     NOT NULL,       -- 关联 lend_gens.gen_id
    order_id            VARCHAR(64)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    amount              DECIMAL(36,18)  NOT NULL,
    kind                VARCHAR(32)     NOT NULL DEFAULT '', -- dup: settle_settle/settle_cancel/cancel_cancel
                                                            -- residue: hold_failed/debit_rolled_back

    -- ── dup 专用字段 ────────────────────────────────────────────────────────
    first_consumed_by   VARCHAR(16)     NOT NULL DEFAULT '',   -- settle | cancel
    first_consumed_at   DATETIME(3)     NULL,
    attempted_by        VARCHAR(16)     NOT NULL DEFAULT '',   -- 本次尝试方
    attempted_at        DATETIME(3)     NULL,

    -- ── residue 专用字段 ─────────────────────────────────────────────────────
    debit_idem_key      VARCHAR(128)    NOT NULL DEFAULT '',   -- 借记幂等键
    compensate_idem_key VARCHAR(128)    NOT NULL DEFAULT '',   -- 补偿贷记幂等键
    fail_reason         VARCHAR(512)    NOT NULL DEFAULT '',   -- hold 失败原因（截断 256 字节后存储）

    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (proof_id),
    UNIQUE KEY uq_lp_gen_type (gen_id, proof_type),    -- 同 gen 同类型至多一条
    KEY idx_lp_order_id (order_id),
    KEY idx_lp_type_created (proof_type, created_at),
    KEY idx_lp_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='放贷双释/半截补偿证明';
