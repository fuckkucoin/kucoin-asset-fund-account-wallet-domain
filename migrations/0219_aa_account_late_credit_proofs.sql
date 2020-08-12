-- 关户迟贷证明（account_late_credit_proofs）。
--
-- 设计约束：
--   - Closed 后观察 / 出箱 / 法币回调贷记必须拒入或改挂争议仓并落证明。
--   - 重开后旧 close_epoch 贷记不得入新活户。
--   - 金额以十进制字符串存储，禁止浮点路径。
--
-- 相位：open → rejected | disputed → closed
--
-- 迁移编号：0219
-- 作者：Span / Likeo
-- 日期：2021.07.10

CREATE TABLE IF NOT EXISTS account_late_credit_proofs (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id          VARCHAR(64)   NOT NULL                       COMMENT '账本用户',
    claimed_user     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '声明用户',
    book             VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '科目',
    source           VARCHAR(32)   NOT NULL                       COMMENT 'observe|outbox|fiat_callback',
    asset            VARCHAR(32)   NOT NULL                       COMMENT '资产代码',
    amount           VARCHAR(64)   NOT NULL                       COMMENT '金额（十进制字符串）',
    tx_ref           VARCHAR(128)  NOT NULL DEFAULT ''            COMMENT '链上或出箱引用',
    claimed_epoch    BIGINT        NOT NULL DEFAULT 0             COMMENT '声明 close_epoch',
    actual_epoch     BIGINT        NOT NULL DEFAULT 0             COMMENT '当前 close_epoch',
    reason           VARCHAR(48)   NOT NULL                       COMMENT '原因码',
    state            VARCHAR(24)   NOT NULL DEFAULT 'open'        COMMENT 'open|rejected|disputed|closed',
    decision         VARCHAR(24)   NOT NULL DEFAULT 'reject'      COMMENT 'admit|reject|dispute',
    idem_key         VARCHAR(256)  NOT NULL                       COMMENT '幂等键',
    digest           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'SHA-256 摘要',
    hold_id          VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '争议仓 ID',
    wrong_user       TINYINT(1)    NOT NULL DEFAULT 0             COMMENT '错户标记',
    force_live       TINYINT(1)    NOT NULL DEFAULT 0             COMMENT '强制入活账（应拒绝）',
    actor            VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作者',
    note             VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '备注',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '更新时刻',
    closed_at        DATETIME(3)   NULL                           COMMENT '闭合时刻',
    UNIQUE KEY uk_alcp_idem (idem_key),
    KEY idx_alcp_user (user_id, created_at),
    KEY idx_alcp_state (state, created_at),
    KEY idx_alcp_source (source, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='关户迟贷 CloseLateCreditProof';

CREATE TABLE IF NOT EXISTS account_late_credit_pending (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id          VARCHAR(64)   NOT NULL                       COMMENT '用户 ID',
    source           VARCHAR(32)   NOT NULL                       COMMENT '来源',
    asset            VARCHAR(32)   NOT NULL                       COMMENT '资产',
    amount           VARCHAR(64)   NOT NULL                       COMMENT '金额',
    tx_ref           VARCHAR(128)  NOT NULL DEFAULT ''            COMMENT '引用',
    claimed_epoch    BIGINT        NULL                           COMMENT '声明世代，可空=缺省 fail-closed',
    seen_at          DATETIME(3)   NOT NULL                       COMMENT '入队时刻',
    scanned          TINYINT(1)    NOT NULL DEFAULT 0             COMMENT '是否已扫描',
    KEY idx_alcpn_scan (scanned, seen_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='迟贷扫描队列';
