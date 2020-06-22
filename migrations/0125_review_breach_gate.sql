-- SLA 违约关案与人工放行夹缝封堵引擎迁移。
--
-- 包含三张表：
--   review_breaches          — SLA 违约挡写世代租约（BreachLease）
--   review_breach_proofs     — 违约证明仓（SlaBreachProof，不可变）
--   review_breach_clearances — 四眼清障工单（BreachClearance）
--
-- 设计原则：
--   1. review_breaches.breach_gen UNIQUE 确保同一 reviewsla.Ticket 的 breach_gen 令牌
--      只能建档一次；InnoDB gap lock 防止并发插入相同 breach_gen。
--   2. review_breaches.state 由应用层状态机控制（blocking→expired_case|cleared）；
--      数据库层不添加触发器；状态变更须先通过 applyBreachEvent 校验。
--   3. review_breach_proofs 为追加型不可变记录；(breach_lease_id, block_outcome) 唯一
--      索引确保同类证明幂等写入（blocked_allow / expired_case / cleared_no_allow）。
--   4. review_breach_clearances 限制同一 breach_lease_id 只能有一个 pending 工单；
--      应用层在 ProposeBreachClearance 中检查并返回 ErrDuplicateBreach。
--   5. 三表均不引用 cases / compliance 外键；隔离边界在应用层保证。
--   6. clearance.no_auto_allow 字段持久化「清障≠放行出金」语义，用于审计追溯。
--   7. review_breaches.case_forced_expired_at 记录 Worker 驱动案件 Expire 的时刻，
--      与 review_breach_proofs.block_outcome='expired_case' 双重证明。
--
-- 迁移标识：0069
-- 作者：Styx
-- 日期：2021.06.20

-- +migrate Up

CREATE TABLE IF NOT EXISTS review_breaches (
    id                      VARCHAR(64)   NOT NULL                    COMMENT 'BreachLease ID（UUID）',
    case_id                 VARCHAR(64)   NOT NULL                    COMMENT '关联审查案件 ID（只读消费 cases.Case）',
    movement_id             VARCHAR(128)  NOT NULL                    COMMENT '关联出金单 ID',
    user_id                 VARCHAR(64)   NOT NULL DEFAULT ''         COMMENT '用户 ID',

    -- 违约世代令牌
    breach_gen              VARCHAR(128)  NOT NULL                    COMMENT '违约世代令牌（来自 reviewsla.Ticket.BreachGen；唯一）',

    -- 违约时间线
    deadline                DATETIME(3)   NOT NULL                    COMMENT 'SLA 截止时刻（UTC）',
    breached_at             DATETIME(3)   NOT NULL                    COMMENT '首次标为 Breached 的时刻（UTC）',

    -- 状态机（blocking|expired_case|cleared）
    state                   VARCHAR(16)   NOT NULL DEFAULT 'blocking'
                                COMMENT 'blocking: 挡写活跃；expired_case: 案件已强制 Expire；cleared: 四眼清障完成',

    -- 案件强制 Expire 时刻（Worker 驱动成功后写入）
    case_forced_expired_at  DATETIME(3)   NULL                        COMMENT 'Worker 强制 Expire 案件时刻（UTC）',

    -- 四眼清障信息（State=cleared 时填充）
    proposer                VARCHAR(128)  NULL                        COMMENT '清障发起人',
    approver                VARCHAR(128)  NULL                        COMMENT '清障批准人（四眼第二人，必须不同于 proposer）',
    cleared_at              DATETIME(3)   NULL                        COMMENT '清障批准时刻（UTC）',

    -- 时间戳
    created_at              DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '建档时刻（UTC）',
    updated_at              DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                ON UPDATE CURRENT_TIMESTAMP(3)       COMMENT '最后更新时刻（UTC）',

    PRIMARY KEY (id),
    UNIQUE KEY  uk_rb_breach_gen              (breach_gen),
    KEY         idx_rb_case_state             (case_id, state),
    KEY         idx_rb_movement_state         (movement_id, state),
    KEY         idx_rb_state_created          (state, created_at),
    KEY         idx_rb_user_state             (user_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='SLA 违约挡写世代租约（breach_gen 签发后建档；blocking 状态封禁 Decide(allow) 与出金 Submit）';


CREATE TABLE IF NOT EXISTS review_breach_proofs (
    id                      VARCHAR(64)   NOT NULL                    COMMENT '违约证明 ID（UUID）',
    case_id                 VARCHAR(64)   NOT NULL                    COMMENT '关联案件 ID',
    breach_lease_id         VARCHAR(64)   NOT NULL                    COMMENT '关联 BreachLease ID',
    movement_id             VARCHAR(128)  NOT NULL DEFAULT ''         COMMENT '关联出金单 ID',

    -- 违约时间线
    deadline                DATETIME(3)   NOT NULL                    COMMENT 'SLA 截止时刻（UTC）',
    breached_at             DATETIME(3)   NOT NULL                    COMMENT '违约发现时刻（UTC）',

    -- 证明语义
    block_outcome           VARCHAR(64)   NOT NULL
                                COMMENT 'blocked_allow: Decide(allow)被拦；expired_case: 案件强制Expire；cleared_no_allow: 四眼清障完成但未放行',
    actor                   VARCHAR(128)  NULL                        COMMENT '触发证明的操作者（system 或 ops 人员 ID）',

    -- 时间戳
    recorded_at             DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '证明落写时刻（UTC）',

    PRIMARY KEY (id),
    UNIQUE KEY  uk_rbp_lease_outcome   (breach_lease_id, block_outcome),
    KEY         idx_rbp_case           (case_id),
    KEY         idx_rbp_recorded       (recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='SLA 违约证明仓（不可变；按 breach_lease_id+block_outcome 幂等写入）';


CREATE TABLE IF NOT EXISTS review_breach_clearances (
    id                      VARCHAR(64)   NOT NULL                    COMMENT '清障工单 ID（UUID）',
    breach_lease_id         VARCHAR(64)   NOT NULL                    COMMENT '关联 BreachLease ID',
    case_id                 VARCHAR(64)   NOT NULL                    COMMENT '关联案件 ID',
    movement_id             VARCHAR(128)  NOT NULL DEFAULT ''         COMMENT '关联出金单 ID',

    -- 四眼人员
    proposer                VARCHAR(128)  NOT NULL                    COMMENT '发起人（必须是 ops 人员，不可为 system）',
    approver                VARCHAR(128)  NULL                        COMMENT '批准/拒绝人（必须不同于 proposer）',
    reason                  TEXT          NOT NULL                    COMMENT '清障原因说明',

    -- 状态机（pending|approved|rejected）
    state                   VARCHAR(16)   NOT NULL DEFAULT 'pending'
                                COMMENT 'pending: 待审批；approved: 已批准；rejected: 已拒绝',

    -- 语义标志
    no_auto_allow           TINYINT(1)    NOT NULL DEFAULT 1          COMMENT '始终为1：清障批准后不隐式放行出金，须重入合规路径',

    -- 时间戳
    proposed_at             DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '发起时刻（UTC）',
    approved_at             DATETIME(3)   NULL                        COMMENT '批准时刻（UTC）',
    rejected_at             DATETIME(3)   NULL                        COMMENT '拒绝时刻（UTC）',

    PRIMARY KEY (id),
    KEY         idx_rbc_lease          (breach_lease_id, state),
    KEY         idx_rbc_case           (case_id),
    KEY         idx_rbc_state_proposed (state, proposed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='SLA 违约四眼清障工单（Proposer+Approver 须不同人；cleared 后不自动放行出金）';

-- +migrate Down

DROP TABLE IF EXISTS review_breach_clearances;
DROP TABLE IF EXISTS review_breach_proofs;
DROP TABLE IF EXISTS review_breaches;
