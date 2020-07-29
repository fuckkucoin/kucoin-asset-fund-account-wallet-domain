-- 限额预检租约表（llg_limit_check_leases）。
--
-- 设计约束：
--   - limitgate.Check 通过后必须落 LimitCheckLease；TTL 内未消费则被 LeaseExpiryWorker 标记 expired。
--   - 占用提交（冻账/出金/划转）必须携带 limit_check_lease_id；缺少 ID 或 lease 过期/吊销
--     → 拒绝放行（fail-closed）。
--   - policy_ver 记录签发时的策略版本号；CommitRecheck 时与当前策略版本，
--     版本变化则置 revoked + 落 LimitDriftProof（policy_flip）。
--   - today_sum_at_issue 记录签发时今日累计金额快照；CommitRecheck 时重新计算，
--     若累计越过 DailyCap 则置 revoked + 落 DriftProof（usage_breach）。
--   - velocity_at_issue 记录签发时速度窗口命中次数；CommitRecheck 时复验，超速则拒绝。
--   - idem_key 唯一，防重复签发同一请求。
--   - commit_ref 绑定占用提交单据 ID（BindLease 时填写）；空表示尚未绑定。
--   - 隔离：llg_ 前缀，与 ptg_（r15 G）/ wvc_（r14 G）键空间完全隔离。
--   - 不存储 availcas 可用 CAS 内核参数；不存储目的地风险图谱限额公式参数。
--
-- 状态机（FSM）：
--   checked ──(BindLease)──▶ bound
--   bound ──(CommitRecheck 通过)──▶ consumed           [终态]
--   checked | bound ──(TTL 超限)──▶ expired            [终态]
--   checked | bound ──(PolicyVer++/TodaySum越阈)──▶ revoked [终态]
--
-- 迁移编号：0198
-- 作者：Span / Likeo / King
-- 日期：2021.12.01

CREATE TABLE IF NOT EXISTS llg_limit_check_leases (
    id                      VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4，limit_check_lease_id',
    user_id                 VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID（操作主体）',
    asset                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码（如 BTC、USDT）',
    kind                    VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '操作类型（withdraw / transfer / deposit）',
    amount                  VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '本次占用请求金额（十进制字符串）',
    dest                    VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '提现目标地址（withdraw 必填；其他可为空）',
    policy_ver              INT           NOT NULL DEFAULT 0             COMMENT '签发时策略版本（固化，CommitRecheck 复验）',
    today_sum_at_issue      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时今日累计金额快照（CommitRecheck 复验）',
    velocity_at_issue       INT           NOT NULL DEFAULT 0             COMMENT '签发时速度窗口命中次数（固化快照）',
    max_single_at_issue     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时单笔限额（固化）',
    daily_cap_at_issue      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时日限额（固化）',
    max_velocity_at_issue   INT           NOT NULL DEFAULT 0             COMMENT '签发时最大速度值（固化）',
    allowlisted             TINYINT(1)    NOT NULL DEFAULT 0             COMMENT '签发时用户是否在白名单（0=false, 1=true）',
    state                   VARCHAR(16)   NOT NULL DEFAULT 'checked'     COMMENT 'checked | bound | consumed | expired | revoked',
    idem_key                VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键（防重复签发）',
    commit_ref              VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '绑定的占用提交单据 ID（BindLease 时写入）',
    issued_at               DATETIME(3)   NOT NULL                       COMMENT '租约签发时刻（Check 通过时刻）',
    expires_at              DATETIME(3)   NOT NULL                       COMMENT '租约过期时刻（LeaseExpiryWorker 清理超 TTL）',
    bound_at                DATETIME(3)                                  COMMENT '占用提交绑定时刻（BindLease 时刻；NULL = 尚未绑定）',
    consumed_at             DATETIME(3)                                  COMMENT '消费完成时刻（CommitRecheck 通过时刻；NULL = 未消费）',
    revoked_at              DATETIME(3)                                  COMMENT '吊销时刻（NULL = 未吊销）',
    revoke_reason           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '吊销原因（policy_flip / usage_breach / velocity_breach / manual / expiry）',
    drift_proof_id          VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 llg_limit_drift_proofs.id（revoked 路径写入）',
    created_at              DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at              DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_llg_leases_idem (idem_key),
    KEY idx_llg_leases_user (user_id, state),
    KEY idx_llg_leases_expires (state, expires_at),
    KEY idx_llg_leases_policy_ver (policy_ver, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='限额预检租约（limit_check_lease）';
