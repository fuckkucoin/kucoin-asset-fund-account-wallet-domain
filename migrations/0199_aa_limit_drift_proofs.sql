-- 限额漂移证明表（llg_limit_drift_proofs）。
--
-- 设计约束：
--   - CommitRecheck 检测到策略版本翻转（policy_flip）、今日累计越阈（usage_breach）
--     或速度越阈（velocity_breach）时必须落证明。
--   - open 状态下，持旧 lease_id 的占用提交一律拒绝并返回 proof_id（ErrProofBlocked）。
--   - proof_hash 全局唯一（SHA-256 hex）；防重复开同一证明。
--   - idem_key 唯一，防并发重复写入。
--   - drift_type 精确标记：policy_flip / usage_breach / velocity_breach。
--   - old_policy_ver / new_policy_ver：policy_flip 时两者均非零；
--     usage_breach / velocity_breach 时两者相同（政策未变，用量越阈）。
--   - old_today_sum / new_today_sum：usage_breach 时记录签发快照与漂移发现时的今日累计差异。
--   - old_velocity / new_velocity：velocity_breach 时记录签发快照与漂移发现时的速度差异。
--   - 不存储 availcas 可用 CAS 内核参数；不存储目的地风险图谱限额公式参数。
--
-- 证明状态机（FSM）：
--   open ──(ProofProveWorker 固化)──▶ proven
--   proven ──(管理员确认)──▶ closed       [终态]
--   open ──(驳回，无需 proven)──▶ closed  [终态]
--
-- 迁移编号：0199
-- 作者：Span / Likeo / King
-- 日期：2021.12.02

CREATE TABLE IF NOT EXISTS llg_limit_drift_proofs (
    id                  VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4，limit_drift_proof_id',
    lease_id            VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '旧 llg_limit_check_leases.id',
    user_id             VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '受影响用户 ID',
    asset               VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码',
    kind                VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '操作类型（withdraw / transfer / deposit）',
    drift_type          VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '漂移类型（policy_flip / usage_breach / velocity_breach）',
    old_policy_ver      INT           NOT NULL DEFAULT 0             COMMENT '租约签发时策略版本（固化）',
    new_policy_ver      INT           NOT NULL DEFAULT 0             COMMENT '漂移发现时策略版本（policy_flip 时非零）',
    old_today_sum       VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '租约签发时今日累计金额（固化快照）',
    new_today_sum       VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '漂移发现时今日累计金额（usage_breach 时非空）',
    daily_cap           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '日限额（证明追溯字段）',
    old_velocity        INT           NOT NULL DEFAULT 0             COMMENT '租约签发时速度（固化快照）',
    new_velocity        INT           NOT NULL DEFAULT 0             COMMENT '漂移发现时速度（velocity_breach 时非零）',
    max_velocity        INT           NOT NULL DEFAULT 0             COMMENT '最大速度值（证明追溯字段）',
    state               VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open | proven | closed',
    proof_hash          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '证明指纹 SHA-256 hex（全局唯一）',
    idem_key            VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键，防重复开证明',
    opened_at           DATETIME(3)   NOT NULL                       COMMENT '证明开启时刻',
    proven_at           DATETIME(3)                                  COMMENT '证明固化时刻（NULL = 未固化）',
    closed_at           DATETIME(3)                                  COMMENT '证明关闭时刻（NULL = 未关闭）',
    closed_by           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '关闭操作人 ID',
    notes               VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '关闭备注',
    created_at          DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at          DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_llg_proofs_hash (proof_hash),
    UNIQUE KEY uk_llg_proofs_idem (idem_key),
    KEY idx_llg_proofs_lease (lease_id),
    KEY idx_llg_proofs_state (state, opened_at),
    KEY idx_llg_proofs_user (user_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='限额漂移拒放行证明（limit_drift_proof）';
