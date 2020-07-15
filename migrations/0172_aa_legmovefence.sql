-- 跨区再平衡腿意图与库存划转半截双移收口引擎：数据库表定义。
--
-- 设计原则：
--   1. georebal_leg_move_fences：每条腿 Enqueue↔TransferInv 共享一行，
--      钉扎 plan_id / leg_id / idem / src / dst / amount 指纹 / 世代；
--      FSM：issued → enqueued → moved | expired | revoked。
--   2. georebal_double_move_proofs：二次划转或 fail-closed 拒转证明，只追加。
--   3. georebal_half_move_cases：Enqueue 成功但 Transfer 失败，或 Transfer 成功但腿未标 done。
--   4. georebal_leg_move_audit：围栏生命周期审计。
--
-- 隔离：
--   - 不改写 r5 多机房隔离再平衡证明表；
--   - 不改写 sweeprun AgreeGate / agreeleasegate 租约表；
--   - 不改写 treasury / trexecgate Plan→Exec 库存租约表。
--
-- 作者：TianpengLan
-- 日期：2020.12.22

CREATE TABLE IF NOT EXISTS georebal_leg_move_fences (
    fence_id              VARCHAR(40)    NOT NULL,
    plan_id               VARCHAR(64)    NOT NULL,
    leg_id                VARCHAR(64)    NOT NULL,
    idempotency_key       VARCHAR(128)   NOT NULL,
    src_region            VARCHAR(32)    NOT NULL,
    dst_region            VARCHAR(32)    NOT NULL,
    asset                 VARCHAR(16)    NOT NULL,
    amount                BIGINT         NOT NULL,
    fingerprint           VARCHAR(64)    NOT NULL,
    generation            BIGINT         NOT NULL DEFAULT 1,
    state                 VARCHAR(16)    NOT NULL DEFAULT 'issued'
                            COMMENT 'issued|enqueued|moved|expired|revoked',
    ttl_seconds           BIGINT         NOT NULL DEFAULT 300,
    issued_at             DATETIME(3)    NOT NULL,
    enqueued_at           DATETIME(3)    NULL,
    moved_at              DATETIME(3)    NULL,
    expired_at            DATETIME(3)    NULL,
    revoked_at            DATETIME(3)    NULL,
    revoke_reason         VARCHAR(256)   NOT NULL DEFAULT '',
    intent_ref            VARCHAR(64)    NOT NULL DEFAULT '',
    transfer_ref          VARCHAR(64)    NOT NULL DEFAULT '',
    move_count            BIGINT         NOT NULL DEFAULT 0,
    created_at            DATETIME(3)    NOT NULL,
    updated_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (fence_id),
    UNIQUE KEY uk_lmf_idem (idempotency_key),
    UNIQUE KEY uk_lmf_leg (plan_id, leg_id),
    INDEX idx_lmf_state (state),
    INDEX idx_lmf_src (src_region),
    INDEX idx_lmf_dst (dst_region),
    INDEX idx_lmf_expire (state, issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='跨区再平衡腿围栏；Enqueue 与 TransferInv 必须共享 leg_move_fence_id。';

CREATE TABLE IF NOT EXISTS georebal_double_move_proofs (
    proof_id              VARCHAR(40)    NOT NULL,
    kind                  VARCHAR(32)    NOT NULL,
    fence_id              VARCHAR(40)    NOT NULL DEFAULT '',
    plan_id               VARCHAR(64)    NOT NULL DEFAULT '',
    leg_id                VARCHAR(64)    NOT NULL DEFAULT '',
    idempotency_key       VARCHAR(128)   NOT NULL DEFAULT '',
    src_region            VARCHAR(32)    NOT NULL DEFAULT '',
    dst_region            VARCHAR(32)    NOT NULL DEFAULT '',
    amount                BIGINT         NOT NULL DEFAULT 0,
    bind_fingerprint      VARCHAR(64)    NOT NULL DEFAULT '',
    live_fingerprint      VARCHAR(64)    NOT NULL DEFAULT '',
    bind_gen              BIGINT         NOT NULL DEFAULT 0,
    live_gen              BIGINT         NOT NULL DEFAULT 0,
    evidence              TEXT           NOT NULL,
    detected_at           DATETIME(3)    NOT NULL,
    exported_at           DATETIME(3)    NULL,
    created_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (proof_id),
    INDEX idx_gdmp_kind (kind),
    INDEX idx_gdmp_fence (fence_id),
    INDEX idx_gdmp_leg (leg_id),
    INDEX idx_gdmp_detected (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='跨区腿二次划转或旧围栏拒转证明，只追加。';

CREATE TABLE IF NOT EXISTS georebal_half_move_cases (
    case_id               VARCHAR(40)    NOT NULL,
    kind                  VARCHAR(32)    NOT NULL
                            COMMENT 'enqueue_no_transfer|transfer_no_done',
    fence_id              VARCHAR(40)    NOT NULL,
    plan_id               VARCHAR(64)    NOT NULL,
    leg_id                VARCHAR(64)    NOT NULL,
    state                 VARCHAR(16)    NOT NULL DEFAULT 'open'
                            COMMENT 'open|resumed|abandoned',
    resume_lock           VARCHAR(64)    NOT NULL DEFAULT '',
    locked_at             DATETIME(3)    NULL,
    reason                VARCHAR(256)   NOT NULL DEFAULT '',
    opened_at             DATETIME(3)    NOT NULL,
    closed_at             DATETIME(3)    NULL,
    actor                 VARCHAR(64)    NOT NULL DEFAULT '',
    created_at            DATETIME(3)    NOT NULL,
    updated_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (case_id),
    INDEX idx_hmc_fence (fence_id, state),
    INDEX idx_hmc_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='跨区腿半截案：可发现、续提互斥、禁止二次 TransferInv。';

CREATE TABLE IF NOT EXISTS georebal_leg_move_audit (
    audit_id              VARCHAR(40)    NOT NULL,
    fence_id              VARCHAR(40)    NOT NULL DEFAULT '',
    plan_id               VARCHAR(64)    NOT NULL DEFAULT '',
    leg_id                VARCHAR(64)    NOT NULL DEFAULT '',
    action                VARCHAR(32)    NOT NULL,
    actor                 VARCHAR(64)    NOT NULL DEFAULT '',
    detail                VARCHAR(512)   NOT NULL DEFAULT '',
    created_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (audit_id),
    INDEX idx_lma_fence (fence_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='腿围栏生命周期审计。';
