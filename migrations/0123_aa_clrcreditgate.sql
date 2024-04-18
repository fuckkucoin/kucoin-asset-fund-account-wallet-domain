-- 清算释放贷记租约夹缝封堵引擎表族。
-- 含：clr_credit_leases（释放贷记租约）、clr_credit_proofs（交收证明）、clr_batch_drain（批次排空状态）。
-- 不含：日终清算应计表、法币轧差编排表、portfolio 科目树。
--
-- 作者：Eco
-- 日期：2025.02.15

-- ── clr_credit_leases ─────────────────────────────────────────────────────────
-- 释放贷记租约：ReleaseWithLease 签发（issued），ApplyWithLease CAS 消费（consumed）。
-- 同 item_id 至多一个 issued 状态 Lease（uq_ccl_item_issued）；消费/过期/废弃后为终态（不可逆）。
-- INSERT IGNORE 保证幂等写入；不提供 DELETE。
CREATE TABLE IF NOT EXISTS clr_credit_leases (
    lease_id            VARCHAR(36)     NOT NULL,
    item_id             VARCHAR(64)     NOT NULL,       -- clearing.Item.ID（外键引用，只读）
    batch_id            VARCHAR(64)     NOT NULL,       -- clearing.Batch.ID
    user_id             VARCHAR(64)     NOT NULL,       -- 受益人 ID
    asset               VARCHAR(16)     NOT NULL,       -- 资产代码（大写）
    amount              DECIMAL(36,18)  NOT NULL,       -- 金额（精确到 18 位）
    state               VARCHAR(16)     NOT NULL DEFAULT 'issued',
        -- issued | consumed | expired | void（终态不可逆）
    actor               VARCHAR(64)     NOT NULL DEFAULT '', -- 签发操作人
    fence_epoch         BIGINT          NOT NULL DEFAULT 1,  -- 行级围栏 epoch
    fence_token         VARCHAR(64)     NOT NULL DEFAULT '', -- 行级围栏 token
    issued_at           DATETIME(3)     NOT NULL,            -- 签发时刻（UTC）
    expires_at          DATETIME(3)     NOT NULL,            -- TTL 截止时刻（UTC）
    consumed_at         DATETIME(3)     NULL,                -- 消费时刻（UTC）
    consumed_by_proof   VARCHAR(36)     NOT NULL DEFAULT '', -- 消费本 Lease 的 ProofID
    expired_at          DATETIME(3)     NULL,                -- 过期标记时刻（UTC）
    void_at             DATETIME(3)     NULL,                -- void 标记时刻（UTC）
    void_reason         VARCHAR(64)     NOT NULL DEFAULT '', -- void 原因
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (lease_id),
    -- 同 item_id 至多一个 issued Lease（利用 partial index 思想；MySQL 用 state 前缀过滤）
    KEY idx_ccl_item_id (item_id),
    KEY idx_ccl_batch_id (batch_id),
    KEY idx_ccl_state_expires (state, expires_at),   -- 过期扫描器使用
    KEY idx_ccl_batch_state (batch_id, state),       -- CloseBatchGuard CountUnconsumedLeases 使用
    KEY idx_ccl_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='clearing 释放贷记租约（clr_credit_lease）';

-- ── clr_credit_proofs ─────────────────────────────────────────────────────────
-- 交收证明（不可变历史记录）：每次 Apply 产生一条；credit_ok 对每个 item_id 唯一。
-- INSERT IGNORE 幂等；不提供 DELETE/UPDATE。
CREATE TABLE IF NOT EXISTS clr_credit_proofs (
    proof_id            VARCHAR(36)     NOT NULL,
    lease_id            VARCHAR(36)     NOT NULL,       -- 触发本证明的 Lease ID
    item_id             VARCHAR(64)     NOT NULL,       -- clearing.Item.ID
    batch_id            VARCHAR(64)     NOT NULL,       -- clearing.Batch.ID
    user_id             VARCHAR(64)     NOT NULL,       -- 受益人 ID
    asset               VARCHAR(16)     NOT NULL,       -- 资产代码
    amount              DECIMAL(36,18)  NOT NULL,       -- 金额
    kind                VARCHAR(20)     NOT NULL,
        -- credit_ok | credit_fail | dup_apply | expired_lease
    detail              VARCHAR(512)    NOT NULL DEFAULT '', -- 补充说明（错误消息或拒绝原因）
    actor               VARCHAR(64)     NOT NULL DEFAULT '', -- 操作人
    at                  DATETIME(3)     NOT NULL,            -- 证明落地时刻（UTC）
    epoch_at_proof      BIGINT          NOT NULL DEFAULT 0,  -- 围栏 epoch（审计溯源）
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (proof_id),
    -- credit_ok 防双贷唯一约束：同 item_id 至多一条 credit_ok 证明
    UNIQUE KEY uq_ccp_item_ok (item_id, kind),              -- 依赖 kind='credit_ok' 最多一条的应用层语义
    KEY idx_ccp_item_id (item_id),
    KEY idx_ccp_batch_id (batch_id),
    KEY idx_ccp_lease_id (lease_id),
    KEY idx_ccp_kind_at (kind, at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='clearing 交收证明（clr_credit_proof）';

-- ── clr_batch_drain ───────────────────────────────────────────────────────────
-- 批次排空状态：CloseBatchGuard 管理；存在未消费 Lease 时进入 draining，
-- 排空完成或超时强制废弃后进入 closed。
CREATE TABLE IF NOT EXISTS clr_batch_drain (
    batch_id            VARCHAR(64)     NOT NULL,           -- clearing.Batch.ID（主键）
    status              VARCHAR(16)     NOT NULL DEFAULT 'pending',
        -- pending | draining | closed
    total_items         INT             NOT NULL DEFAULT 0, -- 批次内总 Item 数
    released_items      INT             NOT NULL DEFAULT 0, -- 已 released 的 Item 数
    applied_items       INT             NOT NULL DEFAULT 0, -- 已有 credit_ok 证明的 Item 数
    unconsumed_leases   INT             NOT NULL DEFAULT 0, -- 当前 issued（未消费、未过期）Lease 数
    in_flight_applies   INT             NOT NULL DEFAULT 0, -- 当前在途 ApplyWithLease 数（近似值）
    excepted_items      INT             NOT NULL DEFAULT 0, -- 已进入异常队列的 Item 数
    drain_started_at    DATETIME(3)     NULL,               -- 排空开始时刻（DrainDraining 时非空）
    drain_timeout_at    DATETIME(3)     NULL,               -- 排空超时时刻
    snapshot_at         DATETIME(3)     NOT NULL,           -- 快照采集时刻
    fence_epoch         BIGINT          NOT NULL DEFAULT 0,
    fence_token         VARCHAR(64)     NOT NULL DEFAULT '',
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (batch_id),
    KEY idx_cbd_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='clearing 批次排空状态（clr_batch_drain）';
