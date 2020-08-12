-- 归集同意租约审计、规则世代与半截计划挡写巡检表。
--
-- 设计约束：
--   - 租约每次状态迁移必须追加审计行；审计只增不改。
--   - 规则世代记录水位参数或规则版本抬升，供漂移吊销泵消费。
--   - 半截计划挡写行记录 planned 后 Exec 被拒或超时未推进，禁止继续放腿。
--   - 不回写 sweeprun 作业金额内核，不回写 sweeppol.Evaluate 公式。
--
-- 迁移编号：0208
-- 作者：Span / Likeo / King
-- 日期：2021.10.16

CREATE TABLE IF NOT EXISTS alg_agree_lease_audit (
    id              VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    lease_id        VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联租约',
    from_state      VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '迁移前状态',
    to_state        VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '迁移后状态',
    actor           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作主体',
    note            VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '说明',
    created_at      DATETIME(3)   NOT NULL                       COMMENT '记录时刻',
    KEY idx_alg_audit_lease (lease_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='归集同意租约审计';

CREATE TABLE IF NOT EXISTS alg_agree_generations (
    id                  VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    scope               VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT 'all | asset | rule',
    chain               VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '链范围，可空',
    asset               VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产范围，可空',
    generation          BIGINT        NOT NULL DEFAULT 0             COMMENT '推进后的规则世代',
    rule_fingerprint    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '推进时规则指纹',
    reason              VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'rule_flip / waterline_raise / bal_cross',
    created_at          DATETIME(3)   NOT NULL                       COMMENT '记录时刻',
    KEY idx_alg_gen_scope (scope, asset),
    KEY idx_alg_gen_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='归集同意规则/水位世代';

CREATE TABLE IF NOT EXISTS alg_agree_half_plans (
    id              VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    lease_id        VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联租约',
    plan_ref        VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '半截计划单据',
    asset           VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码',
    reason          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'stale_plan / exec_refused / drift_block',
    blocked         TINYINT       NOT NULL DEFAULT 1             COMMENT '1=挡写，禁止继续 Exec',
    created_at      DATETIME(3)   NOT NULL                       COMMENT '记录时刻',
    KEY idx_alg_half_lease (lease_id),
    KEY idx_alg_half_plan (plan_ref)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='归集同意半截计划挡写巡检';
