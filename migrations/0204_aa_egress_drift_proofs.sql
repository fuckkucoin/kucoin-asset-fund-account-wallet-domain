-- 出金门控漂移拒广播证明表（elg_egress_drift_proofs）。
--
-- 设计约束：
--   - BroadcastAssert 发现 Flags/Breach/Tank/Halt 世代漂移时必须落证明。
--   - 缺租约广播 fail-closed 时亦可落 missing_lease 证明（幂等）。
--   - open 状态下持旧 egress_lease_id 的广播提交一律拒绝。
--   - 不存储补仓代次锁、不存储热库 cap 公式参数。
--
-- 证明状态机：open → proven → closed；open 可直接驳回至 closed。
--
-- 迁移编号：0204
-- 作者：Span / Likeo / King
-- 日期：2021.11.18

CREATE TABLE IF NOT EXISTS elg_egress_drift_proofs (
    id                      VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    lease_id                VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '旧 elg_egress_leases.id，缺租约可为空',
    chain                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '链标识',
    asset                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码',
    drift_type              VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT 'halt_flip / level_halt / flags_drift / tank_drift / missing_lease / expired',
    old_flags_fingerprint   VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时 Flags 指纹',
    new_flags_fingerprint   VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '复验时 Flags 指纹',
    old_breach              VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '签发时水位',
    new_breach              VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '复验时水位',
    old_tank_digest         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时 Tank 摘要',
    new_tank_digest         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '复验时 Tank 摘要',
    old_halt_generation     BIGINT        NOT NULL DEFAULT 0             COMMENT '签发时停仓世代',
    new_halt_generation     BIGINT        NOT NULL DEFAULT 0             COMMENT '复验时停仓世代',
    state                   VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open | proven | closed',
    proof_hash              VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'SHA-256 hex',
    idem_key                VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键',
    opened_at               DATETIME(3)   NOT NULL                       COMMENT '开启时刻',
    proven_at               DATETIME(3)                                  COMMENT '固化时刻',
    closed_at               DATETIME(3)                                  COMMENT '关闭时刻',
    closed_by               VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '关闭操作人',
    notes                   VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '关闭备注',
    created_at              DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at              DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_elg_proofs_hash (proof_hash),
    UNIQUE KEY uk_elg_proofs_idem (idem_key),
    KEY idx_elg_proofs_lease (lease_id),
    KEY idx_elg_proofs_state (state, opened_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金门控漂移拒广播证明（egress_drift_proof）';
