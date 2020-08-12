-- 超提屏障差异证明表（ow_variance_proofs）。
--
-- 设计约束：
--   - LeaseGen 或 Available 漂移时开启证明（state=open）；同时挡同用户同资产的再次 Commit。
--   - open 状态期间，Engine.CommitBind 对同 (user_id, asset) 返回 ErrProofBlocked。
--   - 四眼确认后 state→closed；挡写解除；关联 OverwdDriftCase 同步关闭。
--   - proof_hash 唯一，防重复固化同一证明（SHA-256 hex）。
--   - live_hash_freeze 标记是否命中 live-hash 冻结路径（只读标记；不在此包自动解冻）。
--   - idem_key 唯一，防重复开证明。
--   - 隔离：ow_ 前缀；不含 availcas CAS 公式字段；不含 hotbound cap 字段。
--
-- 迁移编号：0191
-- 作者：Span / Likeo / King
-- 日期：2022.08.10

CREATE TABLE IF NOT EXISTS ow_variance_proofs (
    id              VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    bind_id         VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '触发漂移的 ow_precheck_binds.id',
    user_id         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '受影响用户 ID',
    asset           VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '币种',
    trigger         VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT 'gen_drift | avail_drift | both_drift',
    bind_lease_gen  BIGINT        NOT NULL DEFAULT 0             COMMENT '绑定时 LeaseGen',
    commit_gen      BIGINT        NOT NULL DEFAULT 0             COMMENT 'Commit 时观测 LeaseGen',
    drift_gen       BIGINT        NOT NULL DEFAULT 0             COMMENT 'commit_gen - bind_lease_gen',
    bind_avail      VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT '绑定时 ExpectedAvail（decimal string）',
    commit_avail    VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT 'Commit 时观测 Available（decimal string）',
    state           VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open | closed',
    proof_hash      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'SHA-256 hex 证明指纹（防篡改）',
    sealed_at       DATETIME(3)   NOT NULL                       COMMENT '证明固化时刻',
    confirmed_by    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '四眼确认人 ID',
    confirmed_at    DATETIME(3)                                  COMMENT '四眼确认时刻（NULL=未确认）',
    close_reason    VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '关闭原因',
    drift_case_id   VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 ow_drift_cases.id',
    live_hash_freeze TINYINT(1)   NOT NULL DEFAULT 0             COMMENT '是否命中 live-hash 冻结路径（只读标记，不自动解冻）',
    idem_key        VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键（防重复开证明）',
    proof_epoch     BIGINT        NOT NULL DEFAULT 0             COMMENT '隔离纪元（毫秒，ow_ 前缀键空间）',
    created_at      DATETIME(3)   NOT NULL,
    updated_at      DATETIME(3)   NOT NULL,

    UNIQUE KEY uk_proof_hash     (proof_hash),
    UNIQUE KEY uk_idem_key       (idem_key),
    KEY idx_state                (state),
    KEY idx_user_asset_state     (user_id, asset, state),
    KEY idx_bind_id              (bind_id),
    KEY idx_drift_case_id        (drift_case_id),
    KEY idx_sealed_at            (sealed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='超提屏障差异证明仓；open 时挡写同用户同资产 Commit；四眼确认后关闭；live_hash_freeze 只读标记不自动解冻';
