-- 杠杆强平世代表（margin_liq_gens）。
--
-- 设计约束：
--   - FreezeForLTV 必须推进 liq_gen 并固化 borrow / collateral / ltv 快照。
--   - Release / CloseBorrow 必须携带匹配世代；无世代或漂移不得进入释放成功路径。
--   - 不存储 LTV 公式参数以外的计算内核，不改写部分扣押残留、放贷挂单双释、loanbook 逾期 FSM。
--
-- 相位（与借款联合）：
--   open → freezing → frozen → releasing → closed；hook 失败 → half_open
--
-- 迁移编号：0209
-- 作者：Span / Likeo / King
-- 日期：2021.11.02

CREATE TABLE IF NOT EXISTS margin_liq_gens (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    borrow_id        VARCHAR(64)   NOT NULL                       COMMENT '借款 ID，唯一',
    user_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    collateral_id    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '抵押 Hold ID 快照',
    gen              BIGINT        NOT NULL DEFAULT 0             COMMENT '单调递增 liq_gen',
    ltv_bps          INT           NOT NULL DEFAULT 0             COMMENT '签发时 LTV 万分比快照（只读）',
    cap_bps          INT           NOT NULL DEFAULT 0             COMMENT '签发时 Cap 快照（只读）',
    borrow_amt       VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '借款金额快照',
    collateral_amt   VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '抵押金额快照',
    asset            VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码',
    phase            VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open|freezing|frozen|releasing|closed|half_open',
    lease_id         VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '当前冻结租约',
    hold_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'KindLiqFreeze Hold',
    idem_key         VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '冻结幂等键',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_mlg_borrow (borrow_id),
    KEY idx_mlg_phase (phase, updated_at),
    KEY idx_mlg_user (user_id, phase)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='杠杆强平世代（liq_gen）';
