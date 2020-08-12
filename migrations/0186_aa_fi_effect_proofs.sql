-- 站内转账效果证明表（fi_effect_proofs）。
--
-- 设计约束：
--   - proof_hash = SHA-256(snap_id ‖ ":" ‖ from_user_id ‖ ":" ‖ to_user_id ‖ ":" ‖ amount ‖ ":" ‖ ledger_id)。
--   - snap_id + ledger_id 唯一（不得重复开证）。
--   - proof_hash 唯一（防双录与重放）。
--   - 证明缺失时禁止将对应 Send 记录标记为 completed。
--   - fence_epoch 用于与 xfer 半截 run_id 键空间隔离（毫秒时间戳，前缀 fi_，不使用 r6 D run_id 格式）。
--
-- 迁移编号：0186
-- 作者：Span / King
-- 日期：2022.06.12

CREATE TABLE IF NOT EXISTS fi_effect_proofs (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY        COMMENT 'UUID v4',
    snap_id         VARCHAR(36)  NOT NULL DEFAULT ''         COMMENT 'fi_resolve_snaps.id',
    lease_id        VARCHAR(36)  NOT NULL DEFAULT ''         COMMENT 'fi_debit_leases.id',
    ledger_id       VARCHAR(128) NOT NULL DEFAULT ''         COMMENT 'ledger.Transfer.ID（SpotSend 单据）',
    from_user_id    VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '发起方用户 ID',
    to_user_id      VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '收款方（快照锁定）',
    asset           VARCHAR(16)  NOT NULL DEFAULT ''         COMMENT '币种',
    amount          VARCHAR(40)  NOT NULL DEFAULT '0'        COMMENT '金额（decimal string）',
    idem_key        VARCHAR(128) NOT NULL DEFAULT ''         COMMENT 'Send 幂等键',
    proof_hash      VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT 'SHA-256 hex（64 位）',
    sealed_at       DATETIME(3)  NOT NULL                    COMMENT '证明固化时刻',
    fence_epoch     BIGINT       NOT NULL DEFAULT 0          COMMENT '隔离纪元（毫秒，fi_ 前缀键空间，≠r6 D run_id）',
    created_at      DATETIME(3)  NOT NULL,

    -- 防重复 / 防重放
    UNIQUE KEY uk_snap_ledger (snap_id, ledger_id),
    UNIQUE KEY uk_proof_hash  (proof_hash),
    KEY idx_lease_id          (lease_id),
    KEY idx_from_user         (from_user_id),
    KEY idx_idem_key          (idem_key),
    KEY idx_sealed_at         (sealed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='站内转账效果证明仓；proof_hash 防重放，缺失则禁 completed；fence_epoch 隔离 r6 D run_id 键空间';

-- SendRecord 业务级追踪表（fi_send_records）。
-- 附属于本迁移，因其生命周期与效果证明强绑定。
CREATE TABLE IF NOT EXISTS fi_send_records (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY        COMMENT 'UUID v4',
    idem_key        VARCHAR(128) NOT NULL DEFAULT ''         COMMENT '幂等键',
    from_user_id    VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '发起方用户 ID',
    asset           VARCHAR(16)  NOT NULL DEFAULT ''         COMMENT '币种',
    amount          VARCHAR(40)  NOT NULL DEFAULT '0'        COMMENT '金额',
    state           VARCHAR(16)  NOT NULL DEFAULT 'resolving' COMMENT 'resolving|debiting|sealing|completed|blocked',
    snap_id         VARCHAR(36)  NOT NULL DEFAULT ''         COMMENT 'fi_resolve_snaps.id',
    lease_id        VARCHAR(36)  NOT NULL DEFAULT ''         COMMENT 'fi_debit_leases.id',
    ledger_id       VARCHAR(128) NOT NULL DEFAULT ''         COMMENT 'ledger.Transfer.ID',
    proof_id        VARCHAR(36)  NOT NULL DEFAULT ''         COMMENT 'fi_effect_proofs.id',
    blocked_at      DATETIME(3)  NULL                        COMMENT 'blocked 时刻',
    block_reason    VARCHAR(512) NOT NULL DEFAULT ''         COMMENT 'blocked 原因',
    created_at      DATETIME(3)  NOT NULL,
    updated_at      DATETIME(3)  NOT NULL,

    UNIQUE KEY uk_idem_key  (idem_key),
    KEY idx_state           (state),
    KEY idx_from_user       (from_user_id),
    KEY idx_updated_at      (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='站内转账业务级追踪；FSM resolving→debiting→sealing→completed/blocked；缺 proof 禁 completed';

