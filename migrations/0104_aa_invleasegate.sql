-- 功能：invleasegate 预检租约与漂移证明表结构。
-- 边界：
--   invcheck_precheck_leases   — DriftGate.Check 放行后签发的预检租约（含建议指纹快照）。
--   invcheck_gate_drift_proofs — 提交路径拒绝时落地的漂移证明（append-only）。
-- 不变量：
--   invcheck_precheck_leases 无 DELETE；状态推进只通过 UPDATE SET state。
--   invcheck_gate_drift_proofs 无 UPDATE（ProofState 更新例外）；只追加证明。
--   同一 lease_id 在 invcheck_gate_drift_proofs 中最多一条（uq_proof_lease_id 保证）。
--   state NOT IN ('committed','expired','blocked') 的租约为活跃租约；业务层保证同资产
--   同时段活跃租约数量在合理范围内（通常≤1，多租约仅在并发预检时短暂存在）。
--
-- 作者：Eco
-- 日期：2020.12.11

-- ── invcheck_precheck_leases ────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS invcheck_precheck_leases (
    lease_id      VARCHAR(64)   NOT NULL                     COMMENT 'inv_precheck_id（UUID）',
    asset         VARCHAR(16)   NOT NULL DEFAULT ''          COMMENT '归属资产（大写，如 BTC）',
    actor         VARCHAR(128)  NOT NULL DEFAULT ''          COMMENT '发起预检的操作人',

    state         VARCHAR(16)   NOT NULL DEFAULT 'issued'
                      COMMENT '租约状态：issued/bound/committed/expired/blocked',

    -- 建议指纹快照（签发时采集）
    fp_has_active     TINYINT(1)   NOT NULL DEFAULT 0        COMMENT '签发时是否存在活跃封堵建议',
    fp_advice_hash    VARCHAR(64)  NOT NULL DEFAULT ''       COMMENT '签发时建议指纹 sha256 hex（无建议为空）',
    fp_captured_at    DATETIME(3)  NOT NULL                  COMMENT '指纹采集时间（UTC）',

    -- 时间轴
    issued_at     DATETIME(3)   NOT NULL                     COMMENT '签发时间（UTC）',
    expires_at    DATETIME(3)   NOT NULL                     COMMENT '到期时间（UTC）',
    bound_at      DATETIME(3)   NULL                         COMMENT '提交路径绑定时间',
    commit_ref    VARCHAR(128)  NOT NULL DEFAULT ''          COMMENT '出金/归集提交业务 ID（仅供追踪）',

    -- 终态时间与原因
    committed_at  DATETIME(3)   NULL                         COMMENT '提交复验通过时间',
    expired_at    DATETIME(3)   NULL                         COMMENT '过期时间',
    blocked_at    DATETIME(3)   NULL                         COMMENT '封堵时间',
    block_reason  VARCHAR(64)   NOT NULL DEFAULT ''
                      COMMENT '封堵原因：advice_flip/checker_down/commit_no_lease/commit_finger_mismatch',

    created_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (lease_id),
    INDEX idx_asset_state      (asset, state),
    INDEX idx_expires_state    (expires_at, state),
    INDEX idx_issued_at        (issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='invcheck 预检租约（DriftGate.Check 放行后签发，TTL 内有效）';

-- ── invcheck_gate_drift_proofs ──────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS invcheck_gate_drift_proofs (
    proof_id      VARCHAR(64)   NOT NULL                     COMMENT '漂移证明 ID（UUID）',
    lease_id      VARCHAR(64)   NOT NULL DEFAULT ''          COMMENT '关联预检租约 ID（可为空，当提交路径无 lease 时）',
    asset         VARCHAR(16)   NOT NULL DEFAULT ''          COMMENT '归属资产',

    kind          VARCHAR(64)   NOT NULL DEFAULT ''
                      COMMENT '漂移类型：advice_flip/checker_down/commit_no_lease/commit_finger_mismatch',
    state         VARCHAR(16)   NOT NULL DEFAULT 'open'
                      COMMENT '证明状态：open/closed',

    -- 建议指纹快照（签发时与提交时核对）
    fp_issue_has_active      TINYINT(1)   NOT NULL DEFAULT 0   COMMENT '签发时是否存在活跃建议',
    fp_issue_advice_hash     VARCHAR(64)  NULL                  COMMENT '签发时建议指纹',
    fp_issue_captured_at     DATETIME(3)  NULL                  COMMENT '签发时指纹采集时间',

    fp_commit_has_active     TINYINT(1)   NOT NULL DEFAULT 0   COMMENT '提交时是否存在活跃建议',
    fp_commit_advice_hash    VARCHAR(64)  NULL                  COMMENT '提交时建议指纹',
    fp_commit_captured_at    DATETIME(3)  NULL                  COMMENT '提交时指纹采集时间',

    -- 操作上下文
    actor         VARCHAR(128)  NOT NULL DEFAULT ''          COMMENT '触发拒绝的操作人',
    commit_ref    VARCHAR(128)  NOT NULL DEFAULT ''          COMMENT '出金/归集提交业务 ID',
    note          VARCHAR(512)  NOT NULL DEFAULT ''          COMMENT '拒绝原因说明',

    closed_at     DATETIME(3)   NULL                         COMMENT '审计关闭时间',
    created_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (proof_id),
    UNIQUE KEY uq_proof_lease_id (lease_id, kind),
    INDEX idx_state            (state),
    INDEX idx_asset            (asset),
    INDEX idx_created          (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='invcheck 偏离闸漂移证明（提交路径拒绝事件，append-only）';
