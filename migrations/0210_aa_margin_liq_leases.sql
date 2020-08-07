-- 杠杆强平冻结意图租约（margin_liq_leases）。
--
-- 设计约束：
--   - 租约固化 borrow_id、collateral_id、liq_gen、ltv 快照与 TTL。
--   - 同一借款同一世代至多一张活跃租约。
--   - Freeze 与 Release 互斥：freezing / releasing 期间对向操作必须失败。
--
-- 状态机：
--   issued → freezing → frozen → releasing → consumed
--   freezing → half_open（hook 失败）
--   非终态 → expired / revoked
--
-- 迁移编号：0210
-- 作者：Span / Likeo
-- 日期：2021.11.06

CREATE TABLE IF NOT EXISTS margin_liq_leases (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4，freeze lease',
    borrow_id        VARCHAR(64)   NOT NULL                       COMMENT '借款 ID',
    user_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    collateral_id    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '抵押 Hold',
    hold_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '强平冻结 Hold',
    liq_gen          BIGINT        NOT NULL DEFAULT 0             COMMENT '签发世代，只读',
    ltv_bps          INT           NOT NULL DEFAULT 0             COMMENT 'LTV 快照',
    cap_bps          INT           NOT NULL DEFAULT 0             COMMENT 'Cap 快照',
    asset            VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产',
    amount           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '冻结金额快照',
    state            VARCHAR(16)   NOT NULL DEFAULT 'issued'      COMMENT 'issued|freezing|frozen|releasing|consumed|expired|revoked|half_open',
    idem_key         VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键',
    issued_at        DATETIME(3)   NOT NULL                       COMMENT '签发时刻',
    expires_at       DATETIME(3)   NOT NULL                       COMMENT 'TTL',
    frozen_at        DATETIME(3)                                  COMMENT 'hook 成功时刻',
    released_at      DATETIME(3)                                  COMMENT '释放闸通过时刻',
    consumed_at      DATETIME(3)                                  COMMENT '关借消费时刻',
    revoked_at       DATETIME(3)                                  COMMENT '吊销时刻',
    revoke_reason    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'ops_force / ttl / race',
    half_proof_id    VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联半截证明',
    race_proof_id    VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联竞态证明',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '行创建',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '行更新',
    UNIQUE KEY uk_mll_idem (idem_key),
    KEY idx_mll_borrow_state (borrow_id, state),
    KEY idx_mll_expires (state, expires_at),
    KEY idx_mll_gen (borrow_id, liq_gen)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='杠杆强平冻结租约';
