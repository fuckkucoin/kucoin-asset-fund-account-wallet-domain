-- 调查案件与溯源快照绑定表（tsg_investigate_case_binds）。
--
-- 设计约束：
--   - 调查案开启时绑定密封快照；一案最多绑定一个快照（case_id 唯一约束）。
--   - snapshot_gen 记录绑定时的快照世代，导出时须复验一致性。
--   - bound_at / bound_by 不可变（绑定后只读）。
--   - 调查导出必须验证对应案件存在有效绑定（缺绑定则拒绝）。
--   - 不存储快照内容；不存储用户账本余额；仅存 ID 关联。
--
-- 迁移编号：0202
-- 作者：Span / Likeo / King
-- 日期：2022.01.12

CREATE TABLE IF NOT EXISTS tsg_investigate_case_binds (
    id              VARCHAR(36)     NOT NULL PRIMARY KEY           COMMENT 'UUID v4，bind_id',
    case_id         VARCHAR(36)     NOT NULL DEFAULT ''            COMMENT '调查案 ID（investigate_cases.id）',
    snapshot_id     VARCHAR(36)     NOT NULL DEFAULT ''            COMMENT '关联 tsg_trace_snapshots.id',
    snapshot_gen    VARCHAR(36)     NOT NULL DEFAULT ''            COMMENT '绑定时的快照世代标识符（复验用）',
    user_id         VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '被调查用户 ID',
    asset           VARCHAR(32)     NOT NULL DEFAULT ''            COMMENT '追踪资产代码',
    bound_at        DATETIME(3)     NOT NULL                       COMMENT '绑定时刻',
    bound_by        VARCHAR(64)     NOT NULL DEFAULT ''            COMMENT '执行绑定的操作员 ID',
    created_at      DATETIME(3)     NOT NULL                       COMMENT '行创建时刻',
    UNIQUE KEY uk_tsg_binds_case (case_id),
    KEY idx_tsg_binds_snap (snapshot_id),
    KEY idx_tsg_binds_snap_gen (snapshot_gen),
    KEY idx_tsg_binds_user (user_id, asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='调查案件与溯源快照绑定（investigate_case_bind）';
