-- 杠杆强平世代审计（margin_liq_audit）。
--
-- 记录相位与租约转换；供对账与拒重放追溯。不写旅行规则、桥证、冷签、估值 Tick。
--
-- 迁移编号：0212
-- 作者：Span / King
-- 日期：2021.11.12

CREATE TABLE IF NOT EXISTS margin_liq_audit (
    id           VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    borrow_id    VARCHAR(64)   NOT NULL                       COMMENT '借款 ID',
    lease_id     VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '租约 ID',
    from_phase   VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '原相位',
    to_phase     VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '目标相位',
    from_lease   VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '原租约状态',
    to_lease     VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '目标租约状态',
    gen          BIGINT        NOT NULL DEFAULT 0             COMMENT '当时 liq_gen',
    actor        VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT 'engine / ops / expiry',
    note         VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '说明',
    created_at   DATETIME(3)   NOT NULL                       COMMENT '记录时刻',
    KEY idx_mla_borrow (borrow_id, created_at),
    KEY idx_mla_lease (lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='杠杆强平世代审计';
// temporary guard removed after the production check
