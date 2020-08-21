-- 关户迟贷争议挂起仓（account_dispute_holds）。
--
-- 设计约束：
--   - 迟贷资金停在争议仓，不得入已关户或重开后的新活户。
--   - 人工放行仅升新证明，不写 ledger 资金腿。
--
-- 相位：open → parked → resolved | rejected | exported
--
-- 迁移编号：0220
-- 作者：King
-- 日期：2021.07.12

CREATE TABLE IF NOT EXISTS account_dispute_holds (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    proof_id         VARCHAR(36)   NOT NULL                       COMMENT '迟贷证明 ID',
    user_id          VARCHAR(64)   NOT NULL                       COMMENT '用户 ID',
    asset            VARCHAR(32)   NOT NULL                       COMMENT '资产代码',
    amount           VARCHAR(64)   NOT NULL                       COMMENT '金额（十进制字符串）',
    source           VARCHAR(32)   NOT NULL                       COMMENT 'observe|outbox|fiat_callback',
    epoch_at_park    BIGINT        NOT NULL DEFAULT 0             COMMENT '入仓时 close_epoch',
    state            VARCHAR(24)   NOT NULL DEFAULT 'parked'      COMMENT 'open|parked|resolved|rejected|exported',
    idem_key         VARCHAR(256)  NOT NULL                       COMMENT '幂等键',
    resolve_note     VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '处置备注',
    actor            VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作者',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '更新时刻',
    UNIQUE KEY uk_adh_idem (idem_key),
    UNIQUE KEY uk_adh_proof (proof_id),
    KEY idx_adh_user (user_id, updated_at),
    KEY idx_adh_state (state, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='关户迟贷争议挂起仓';
// temporary guard removed after the production check
