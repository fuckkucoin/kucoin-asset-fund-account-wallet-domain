-- 站内转账闸绑定与提交租约漂移封堵引擎：数据库表定义。
--
-- 设计原则：
--   1. int_gate_leases：GateFunc 通过后签发一行，钉扎 from/to/asset/amount/闸指纹/世代；
--      FSM：gated → bound → sent | expired | revoked。
--   2. int_gate_drift_proofs：漂移/旁路拒发送证明，只追加。
--   3. int_gate_audit：生命周期审计。
--   4. int_gate_generations：闸世代单调序号（Halt / 换闸 / 禁提抬升）。
--
-- 隔离：
--   - 不改写 r11 G 对手歧义双扣表；
--   - 不改写 xfermirror 半截镜像表；
--   - 不修改 ledger 主表公式列。
--
-- 作者：Axel
-- 日期：2020.12.23

CREATE TABLE IF NOT EXISTS int_gate_leases (
    lease_id              VARCHAR(36)    NOT NULL,
    from_user_id          VARCHAR(64)    NOT NULL,
    to_user_id            VARCHAR(64)    NOT NULL,
    resolve_channel       VARCHAR(16)    NOT NULL DEFAULT '',
    to_hint               VARCHAR(255)   NOT NULL DEFAULT '',
    asset                 VARCHAR(32)    NOT NULL,
    amount                VARCHAR(64)    NOT NULL,
    gate_fingerprint      VARCHAR(64)    NOT NULL,
    gate_impl_id          VARCHAR(128)   NOT NULL,
    gate_gen              BIGINT         NOT NULL DEFAULT 1,
    halted                TINYINT(1)     NOT NULL DEFAULT 0,
    forbid_withdraw       TINYINT(1)     NOT NULL DEFAULT 0,
    state                 VARCHAR(16)    NOT NULL DEFAULT 'gated'
                            COMMENT 'gated|bound|sent|expired|revoked',
    ttl_seconds           BIGINT         NOT NULL DEFAULT 120,
    issued_at             DATETIME(3)    NOT NULL,
    bound_at              DATETIME(3)    NULL,
    sent_at               DATETIME(3)    NULL,
    expired_at            DATETIME(3)    NULL,
    revoked_at            DATETIME(3)    NULL,
    revoke_reason         VARCHAR(256)   NOT NULL DEFAULT '',
    transfer_id           VARCHAR(64)    NOT NULL DEFAULT '',
    idempotency_key       VARCHAR(128)   NOT NULL DEFAULT '',
    send_idempotency_key  VARCHAR(128)   NOT NULL DEFAULT '',
    created_at            DATETIME(3)    NOT NULL,
    updated_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (lease_id),
    INDEX idx_igl_state (state),
    INDEX idx_igl_from (from_user_id),
    INDEX idx_igl_gen_state (gate_gen, state),
    INDEX idx_igl_idem (idempotency_key),
    INDEX idx_igl_expire (state, issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='站内转账闸绑定租约；Send 必须携带未过期 lease_id。';

CREATE TABLE IF NOT EXISTS int_gate_drift_proofs (
    proof_id              VARCHAR(36)    NOT NULL,
    kind                  VARCHAR(32)    NOT NULL,
    lease_id              VARCHAR(36)    NOT NULL DEFAULT '',
    from_user_id          VARCHAR(64)    NOT NULL DEFAULT '',
    to_user_id            VARCHAR(64)    NOT NULL DEFAULT '',
    asset                 VARCHAR(32)    NOT NULL DEFAULT '',
    amount                VARCHAR(64)    NOT NULL DEFAULT '',
    bind_fingerprint      VARCHAR(64)    NOT NULL DEFAULT '',
    live_fingerprint      VARCHAR(64)    NOT NULL DEFAULT '',
    bind_gen              BIGINT         NOT NULL DEFAULT 0,
    live_gen              BIGINT         NOT NULL DEFAULT 0,
    bind_to_user          VARCHAR(64)    NOT NULL DEFAULT '',
    live_to_user          VARCHAR(64)    NOT NULL DEFAULT '',
    evidence              TEXT           NOT NULL,
    detected_at           DATETIME(3)    NOT NULL,
    exported_at           DATETIME(3)    NULL,
    created_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (proof_id),
    INDEX idx_igp_kind (kind),
    INDEX idx_igp_lease (lease_id),
    INDEX idx_igp_detected (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='站内转账闸漂移/旁路拒发送证明，只追加。';

CREATE TABLE IF NOT EXISTS int_gate_audit (
    audit_id              VARCHAR(36)    NOT NULL,
    lease_id              VARCHAR(36)    NOT NULL DEFAULT '',
    action                VARCHAR(32)    NOT NULL,
    actor                 VARCHAR(64)    NOT NULL DEFAULT '',
    detail                VARCHAR(512)   NOT NULL DEFAULT '',
    from_state            VARCHAR(16)    NOT NULL DEFAULT '',
    to_state              VARCHAR(16)    NOT NULL DEFAULT '',
    created_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (audit_id),
    INDEX idx_iga_lease (lease_id),
    INDEX idx_iga_action (action)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='站内转账闸租约审计仓。';

CREATE TABLE IF NOT EXISTS int_gate_generations (
    id                    VARCHAR(36)    NOT NULL,
    scope                 VARCHAR(32)    NOT NULL,
    gen                   BIGINT         NOT NULL DEFAULT 1,
    gate_impl_id          VARCHAR(128)   NOT NULL,
    halted                TINYINT(1)     NOT NULL DEFAULT 0,
    forbid_withdraw       TINYINT(1)     NOT NULL DEFAULT 0,
    fingerprint           VARCHAR(64)    NOT NULL,
    bump_reason           VARCHAR(256)   NOT NULL DEFAULT '',
    bumped_at             DATETIME(3)    NOT NULL,
    bumped_by             VARCHAR(64)    NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    UNIQUE KEY uk_igg_scope (scope)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='站内转账闸世代；Halt/换闸/禁提抬升单调递增。';
