-- 托管熔断清闸四眼协议表族。
-- 含：halt_clear_plans（四眼提案）、halt_close_proofs（闭合证明）、misclear_cases（误清案例）、reengage_legs（再熔断腿）。
-- 不含战争室级联冻结计划（freeze_plans）；与 r7 D 正交。
-- clear_plan_id / misclear_id 供只读对账路径关联。
--
-- 作者：Eco
-- 日期：2025.11.14

-- ---- halt_clear_plans ----
-- 清闸四眼提案；状态机：proposed → quorum → cleared / rejected。
-- plan_id 全局唯一；proposer / approver1 / approver2 须为不同操作人。
-- freeze_war_room_plan_id 只读关联，本包不写级联冻结计划。
CREATE TABLE IF NOT EXISTS halt_clear_plans (
    plan_id              VARCHAR(128)    NOT NULL,
    scope                VARCHAR(32)     NOT NULL DEFAULT 'global',
    chain                VARCHAR(32)     NOT NULL DEFAULT '',
    asset                VARCHAR(32)     NOT NULL DEFAULT '',
    proposer             VARCHAR(256)    NOT NULL,
    reason               VARCHAR(1024)   NOT NULL DEFAULT '',
    state                VARCHAR(32)     NOT NULL DEFAULT 'proposed',
    approver1            VARCHAR(256)    NOT NULL DEFAULT '',
    approver1_at         DATETIME(3)     NULL,
    approver2            VARCHAR(256)    NOT NULL DEFAULT '',
    approver2_at         DATETIME(3)     NULL,
    reject_reason        VARCHAR(1024)   NOT NULL DEFAULT '',
    rejected_by          VARCHAR(256)    NOT NULL DEFAULT '',
    rejected_at          DATETIME(3)     NULL,
    close_proof_id       VARCHAR(256)    NOT NULL DEFAULT '',
    open_diff_count      INT             NOT NULL DEFAULT 0,
    proposed_at          DATETIME(3)     NOT NULL,
    cleared_at           DATETIME(3)     NULL,
    freeze_war_room_plan_id VARCHAR(256) NOT NULL DEFAULT '',
    created_at           DATETIME(3)     NOT NULL,
    updated_at           DATETIME(3)     NOT NULL,
    PRIMARY KEY (plan_id),
    KEY idx_hcp_state (state),
    KEY idx_hcp_proposer_state (proposer, state),
    KEY idx_hcp_proposed_at (proposed_at)
);

-- ---- halt_close_proofs ----
-- 清闸闭合证明；四眼 + 零差异的不可变快照，供事后审计。
-- 一旦写入不得修改；open_diff_count normal 时必须为 0；forced 时记录实测值。
CREATE TABLE IF NOT EXISTS halt_close_proofs (
    proof_id             VARCHAR(256)    NOT NULL,
    plan_id              VARCHAR(128)    NOT NULL,
    proof_type           VARCHAR(32)     NOT NULL DEFAULT 'normal',
    scope                VARCHAR(32)     NOT NULL DEFAULT 'global',
    chain                VARCHAR(32)     NOT NULL DEFAULT '',
    asset                VARCHAR(32)     NOT NULL DEFAULT '',
    approver1            VARCHAR(256)    NOT NULL,
    approver2            VARCHAR(256)    NOT NULL,
    open_diff_count      INT             NOT NULL DEFAULT 0,
    issued_at            DATETIME(3)     NOT NULL,
    diff_summary         TEXT            NOT NULL DEFAULT '',
    checksum             VARCHAR(64)     NOT NULL DEFAULT '',
    PRIMARY KEY (proof_id),
    KEY idx_hcpr_plan_id (plan_id),
    KEY idx_hcpr_issued_at (issued_at)
);

-- ---- misclear_cases ----
-- 误清案例：已解除熔断但仍存在未关差异的异常状态。
-- 状态机：open → reengaged → closed。
-- misclear_id 全局唯一；forced_audit_id / clear_plan_id 只读关联，不含金额。
CREATE TABLE IF NOT EXISTS misclear_cases (
    misclear_id          VARCHAR(256)    NOT NULL,
    scope                VARCHAR(32)     NOT NULL DEFAULT 'global',
    chain                VARCHAR(32)     NOT NULL DEFAULT '',
    asset                VARCHAR(32)     NOT NULL DEFAULT '',
    trigger              VARCHAR(64)     NOT NULL DEFAULT 'open_diff_scan',
    state                VARCHAR(32)     NOT NULL DEFAULT 'open',
    detected_at          DATETIME(3)     NOT NULL,
    open_diff_count      INT             NOT NULL DEFAULT 0,
    open_diff_keys       TEXT            NOT NULL DEFAULT '',   -- JSON 数组，最多 32 条
    forced_audit_id      VARCHAR(256)    NOT NULL DEFAULT '',
    clear_plan_id        VARCHAR(128)    NOT NULL DEFAULT '',
    reengaged_at         DATETIME(3)     NULL,
    reengage_actor_id    VARCHAR(256)    NOT NULL DEFAULT '',
    reengage_leg_id      VARCHAR(256)    NOT NULL DEFAULT '',
    closed_at            DATETIME(3)     NULL,
    closed_by            VARCHAR(256)    NOT NULL DEFAULT '',
    close_note           VARCHAR(2048)   NOT NULL DEFAULT '',
    created_at           DATETIME(3)     NOT NULL,
    updated_at           DATETIME(3)     NOT NULL,
    PRIMARY KEY (misclear_id),
    KEY idx_mc_scope_state (scope, state),
    KEY idx_mc_state_detected (state, detected_at),
    KEY idx_mc_detected_at (detected_at),
    KEY idx_mc_forced_audit_id (forced_audit_id),
    KEY idx_mc_clear_plan_id (clear_plan_id)
);

-- ---- reengage_legs ----
-- 再熔断腿：误清确认后自动/半自动再熔断执行记录与回滚证明。
-- 状态机：pending → executing → engaged / failed。
-- rollback_proof 存储 JSON 序列化的 RollbackProof；engaged 后不可修改。
CREATE TABLE IF NOT EXISTS reengage_legs (
    leg_id               VARCHAR(256)    NOT NULL,
    misclear_id          VARCHAR(256)    NOT NULL,
    scope                VARCHAR(32)     NOT NULL DEFAULT 'global',
    chain                VARCHAR(32)     NOT NULL DEFAULT '',
    asset                VARCHAR(32)     NOT NULL DEFAULT '',
    state                VARCHAR(32)     NOT NULL DEFAULT 'pending',
    actor_id             VARCHAR(256)    NOT NULL,
    reason               VARCHAR(1024)   NOT NULL DEFAULT '',
    is_auto              TINYINT(1)      NOT NULL DEFAULT 0,
    scheduled_at         DATETIME(3)     NOT NULL,
    executed_at          DATETIME(3)     NULL,
    engaged_at           DATETIME(3)     NULL,
    failed_at            DATETIME(3)     NULL,
    fail_reason          VARCHAR(2048)   NOT NULL DEFAULT '',
    rollback_proof       TEXT            NULL,    -- JSON 序列化的 RollbackProof
    created_at           DATETIME(3)     NOT NULL,
    updated_at           DATETIME(3)     NOT NULL,
    PRIMARY KEY (leg_id),
    KEY idx_rl_misclear_id (misclear_id),
    KEY idx_rl_misclear_state (misclear_id, state),
    KEY idx_rl_state_scheduled (state, scheduled_at),
    KEY idx_rl_created_at (created_at)
);

-- ---- halt_forced_audits ----
-- 强制清闸审计记录；ClearForced 必须写入，缺理由码拒绝。
-- 与 misclear_cases 通过 forced_audit_id 关联。
CREATE TABLE IF NOT EXISTS halt_forced_audits (
    audit_id             VARCHAR(256)    NOT NULL,
    scope                VARCHAR(32)     NOT NULL DEFAULT 'global',
    chain                VARCHAR(32)     NOT NULL DEFAULT '',
    asset                VARCHAR(32)     NOT NULL DEFAULT '',
    actor                VARCHAR(256)    NOT NULL,
    reason               VARCHAR(1024)   NOT NULL DEFAULT '',
    reason_code          VARCHAR(64)     NOT NULL,
    post_ticket_id       VARCHAR(256)    NOT NULL DEFAULT '',
    open_diff_count      INT             NOT NULL DEFAULT 0,
    flag_snapshot        TEXT            NOT NULL DEFAULT '',
    misclear_risk        TINYINT(1)      NOT NULL DEFAULT 0,
    issued_at            DATETIME(3)     NOT NULL,
    created_at           DATETIME(3)     NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_hfa_scope_actor (scope, actor),
    KEY idx_hfa_reason_code (reason_code),
    KEY idx_hfa_issued_at (issued_at)
);
