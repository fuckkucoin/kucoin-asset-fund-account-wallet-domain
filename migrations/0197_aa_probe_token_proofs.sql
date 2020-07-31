-- 探针令牌漂移证明表与写作用域绑定表。
--
-- 本文件包含：
--   1. ptg_probe_token_proofs  — 探针令牌漂移拒放行证明（ProbeTokenDriftProof）
--   2. ptg_write_scope_binds   — 写作用域绑定记录（WriteScopeBind）
--
-- ── ptg_probe_token_proofs 设计约束 ──────────────────────────────────────
--   - CommitCheck 检测到探针翻转（probe_flip）或毒价撤令（poison_revoke）时必须落证明。
--   - open 状态下，持旧 lease_id 的写提交一律拒绝并返回 proof_id（ErrProofBlocked）。
--   - proof_hash 全局唯一（SHA-256 hex）；防重复开同一证明。
--   - idem_key 唯一，防并发重复写入。
--   - drift_type 精确标记：probe_flip / poison_revoke / token_advance。
--   - old_probe_fingerprint / new_probe_fingerprint：probe_flip 时两者均非空；
--     poison_revoke / token_advance 时 new_probe_fingerprint 与旧值相同（令牌维度漂移）。
--   - old_token_generation / new_token_generation：令牌维度漂移时两者不同；
--     probe_flip 时两者相同（令牌未变，探针翻转）。
--   - 不存储 pricepoison 分级公式参数；不存储 custodyhalt 清闸时间戳。
--
-- ── ptg_write_scope_binds 设计约束 ─────────────────────────────────────
--   - BindLease 成功后写入，记录 lease_id ↔ commit_ref 一对一映射。
--   - (lease_id, commit_ref) 联合唯一。
--   - ConsumedAt 非空时表示 CommitCheck 通过，写提交最终成功。
--
-- 迁移编号：0197
-- 作者：Span / Likeo / King
-- 日期：2021.11.02

-- ── 漂移证明表 ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ptg_probe_token_proofs (
    id                      VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4，probe_token_drift_proof_id',
    lease_id                VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '旧 ptg_probe_token_leases.id',
    scope_key               VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '写作用域键',
    subject_id              VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作主体 ID',
    drift_type              VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '漂移类型（probe_flip / poison_revoke / token_advance）',
    old_probe_fingerprint   VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '租约签发时探针指纹',
    new_probe_fingerprint   VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '漂移发现时探针指纹（probe_flip 时非空）',
    old_token_generation    BIGINT        NOT NULL DEFAULT 0             COMMENT '租约签发时令牌世代',
    new_token_generation    BIGINT        NOT NULL DEFAULT 0             COMMENT '漂移发现时令牌世代（token_advance/poison_revoke 时非空）',
    poison_revoked          TINYINT(1)    NOT NULL DEFAULT 0             COMMENT '是否因毒价撤令触发（0=false, 1=true）',
    state                   VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open | proven | closed',
    proof_hash              VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '证明指纹 SHA-256 hex（全局唯一）',
    idem_key                VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键，防重复开证明',
    opened_at               DATETIME(3)   NOT NULL                       COMMENT '证明开启时刻',
    proven_at               DATETIME(3)                                  COMMENT '证明固化时刻（NULL = 未固化）',
    closed_at               DATETIME(3)                                  COMMENT '证明关闭时刻（NULL = 未关闭）',
    closed_by               VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '关闭操作人 ID',
    notes                   VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '关闭备注',
    created_at              DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at              DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_ptg_proofs_hash (proof_hash),
    UNIQUE KEY uk_ptg_proofs_idem (idem_key),
    KEY idx_ptg_proofs_lease (lease_id),
    KEY idx_ptg_proofs_state (state, opened_at),
    KEY idx_ptg_proofs_subject (subject_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='探针令牌漂移拒放行证明（probe_token_drift_proof）';

-- ── 写作用域绑定表 ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ptg_write_scope_binds (
    id              VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    lease_id        VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 ptg_probe_token_leases.id',
    commit_ref      VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '写提交单据 ID（出金/划转/结算批次 ID）',
    scope_key       VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '写作用域键（冗余存储，供查询）',
    subject_id      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作主体 ID（冗余存储）',
    bound_at        DATETIME(3)   NOT NULL                       COMMENT '绑定时刻（BindLease 时刻）',
    consumed_at     DATETIME(3)                                  COMMENT '消费完成时刻（CommitCheck 通过后写入；NULL = 尚未完成）',
    created_at      DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at      DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_ptg_binds_lease (lease_id),
    UNIQUE KEY uk_ptg_binds_commit (commit_ref),
    KEY idx_ptg_binds_subject (subject_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='写作用域绑定记录（write_scope_bind）';
