-- 探针令牌联合租约表（ptg_probe_token_leases）。
--
-- 设计约束：
--   - writegate.Check 通过后必须落 ProbeTokenLease；TTL 内未消费则被 LeaseExpiryWorker 标记 expired。
--   - 写提交必须携带 probe_token_lease_id；缺少 ID 或 lease 过期/吊销 → 拒绝放行（fail-closed）。
--   - probe_fingerprint 记录签发时的探针互斥状态指纹；CommitCheck 时与当前探针指纹，
--     不匹配则置 revoked + 落 ProbeTokenDriftProof（probe_flip）。
--   - token_generation 记录签发时的令牌世代；CommitCheck 时复验，世代抬升或撤令则拒绝。
--   - idem_key 唯一，防重复签发同一请求。
--   - commit_ref 绑定写提交单据 ID（BindLease 时填写）；空表示尚未绑定。
--   - snap_fingerprint 联合快照指纹（WriteGateSnap.SnapFingerprint）；供审计追溯。
--   - 隔离：ptg_ 前缀，与 wdvc_（r14 G）/ ow_（r13 G）键空间完全隔离。
--   - 不存储 pricepoison PoisonEngine 内核参数；不存储 custodyhalt 清闸时间戳。
--
-- 状态机（FSM）：
--   checked ──(BindLease)──▶ bound
--   bound ──(CommitCheck 通过)──▶ consumed            [终态]
--   checked | bound ──(TTL 超限)──▶ expired            [终态]
--   checked | bound ──(探针翻转/毒价撤令)──▶ revoked   [终态]
--
-- 迁移编号：0196
-- 作者：Span / Likeo / King
-- 日期：2021.11.01

CREATE TABLE IF NOT EXISTS ptg_probe_token_leases (
    id                  VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4，probe_token_lease_id',
    scope_key           VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '写作用域键（Kind|SubjectID|Chain|Asset）',
    scope_kind          VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '写作用域类型（withdraw/transfer/settle/redeem）',
    subject_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作主体 ID（用户 ID / 账户 ID）',
    chain               VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '目标链（出金场景必填；划转可为空）',
    asset               VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码（如 BTC、USDT）',
    probe_fingerprint   VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '探针状态指纹 SHA-256 hex（签发时固化，CommitCheck 复验）',
    probe_pair_id       VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '探针对 ID（签发时固化；IsProbing=false 时为空）',
    is_probing_at_issue TINYINT(1)    NOT NULL DEFAULT 0             COMMENT '签发时探针是否活跃（0=false, 1=true）',
    token_generation    BIGINT        NOT NULL DEFAULT 0             COMMENT '令牌世代（签发时固化，CommitCheck 复验）',
    token_hash          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '令牌指纹 SHA-256 hex（签发时固化）',
    snap_fingerprint    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '联合快照指纹（WriteGateSnap.SnapFingerprint）',
    state               VARCHAR(16)   NOT NULL DEFAULT 'checked'     COMMENT 'checked | bound | consumed | expired | revoked',
    idem_key            VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键（防重复签发）',
    commit_ref          VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '绑定的写提交单据 ID（BindLease 时写入）',
    issued_at           DATETIME(3)   NOT NULL                       COMMENT '租约签发时刻（CheckGate 通过时刻）',
    expires_at          DATETIME(3)   NOT NULL                       COMMENT '租约过期时刻（LeaseExpiryWorker 清理超 TTL）',
    bound_at            DATETIME(3)                                  COMMENT '写提交绑定时刻（BindLease 时刻；NULL = 尚未绑定）',
    consumed_at         DATETIME(3)                                  COMMENT '消费完成时刻（CommitCheck 通过时刻；NULL = 未消费）',
    revoked_at          DATETIME(3)                                  COMMENT '吊销时刻（NULL = 未吊销）',
    revoke_reason       VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '吊销原因（probe_flip / poison_revoke / manual / expiry）',
    drift_proof_id      VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 ptg_probe_token_proofs.id（revoked 路径写入）',
    created_at          DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at          DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_ptg_leases_idem (idem_key),
    KEY idx_ptg_leases_scope (scope_key, state),
    KEY idx_ptg_leases_subject (subject_id, state),
    KEY idx_ptg_leases_expires (state, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='探针令牌联合租约（probe_token_lease）';
