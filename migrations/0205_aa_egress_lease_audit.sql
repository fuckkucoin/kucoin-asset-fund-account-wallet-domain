-- 出金门控租约审计与世代表（elg_egress_lease_audit / elg_egress_generations）。
--
-- 设计约束：
--   - 租约每次状态迁移必须追加审计行；审计只增不改。
--   - 世代行记录 Halt 翻转或水位跌破巡检推进的 scope 世代，供吊销泵消费。
--   - 不回写 floatdesk.Flags 内核，不回写 PlanRefill。
--
-- 迁移编号：0205
-- 作者：Span / Likeo / King
-- 日期：2021.11.20

CREATE TABLE IF NOT EXISTS elg_egress_lease_audit (
    id              VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    lease_id        VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联租约',
    from_state      VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '迁移前状态',
    to_state        VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '迁移后状态',
    actor           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作主体',
    note            VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '说明',
    created_at      DATETIME(3)   NOT NULL                       COMMENT '记录时刻',
    KEY idx_elg_audit_lease (lease_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金门控租约审计';

CREATE TABLE IF NOT EXISTS elg_egress_generations (
    id                  VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    scope               VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT 'all | chain | asset | level',
    chain               VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '链范围，all 为空',
    asset               VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产范围，可空',
    generation          BIGINT        NOT NULL DEFAULT 0             COMMENT '推进后的世代值',
    flags_fingerprint   VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '推进时 Flags 指纹',
    reason              VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'halt_all / halt_chain / halt_asset / level_halt',
    created_at          DATETIME(3)   NOT NULL                       COMMENT '记录时刻',
    KEY idx_elg_gen_scope (scope, chain, asset),
    KEY idx_elg_gen_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金门控停仓/水位世代';
