-- 白名单条目生命周期与出金提交租约收口引擎：数据库表定义。
--
-- 设计原则：
--   1. dest_entry_leases：Precheck/RequireStrict 通过后签发一行，钉扎
--      entry_id / entry_gen / user / chain / addr / mode / TTL；
--      FSM：issued → bound → consumed | expired | revoked。
--   2. dest_entry_drift_proofs：条目漂移 / 缺租约拒出金证明，只追加。
--   3. dest_entry_audit：生命周期审计。
--   4. dest_entry_generations：条目级世代（Temp 过期 / 破窗收回 / Revoke / Restore 未满）。
--
-- 隔离：
--   - 不改写 r8 C 冷却编排表；
--   - 不改写 destlife Inflight 引用撤销表；
--   - 不改写 wdtriplegate ListGen / PolicyVer / KYTPin 三联租约表。
--
-- 作者：TianpengLan
-- 日期：2020.12.16

CREATE TABLE IF NOT EXISTS dest_entry_leases (
    lease_id              VARCHAR(36)    NOT NULL,
    entry_id              VARCHAR(64)    NOT NULL,
    entry_gen             BIGINT         NOT NULL DEFAULT 1,
    user_id               VARCHAR(64)    NOT NULL,
    chain                 VARCHAR(32)    NOT NULL,
    address               VARCHAR(128)   NOT NULL,
    mode                  VARCHAR(16)    NOT NULL DEFAULT 'strict'
                            COMMENT 'strict|temp|breakglass',
    whitelist_only        TINYINT(1)     NOT NULL DEFAULT 1,
    cooldown_full         TINYINT(1)     NOT NULL DEFAULT 1,
    state                 VARCHAR(16)    NOT NULL DEFAULT 'issued'
                            COMMENT 'issued|bound|consumed|expired|revoked',
    ttl_seconds           BIGINT         NOT NULL DEFAULT 180,
    issued_at             DATETIME(3)    NOT NULL,
    bound_at              DATETIME(3)    NULL,
    consumed_at           DATETIME(3)    NULL,
    expired_at            DATETIME(3)    NULL,
    revoked_at            DATETIME(3)    NULL,
    revoke_reason         VARCHAR(256)   NOT NULL DEFAULT '',
    withdraw_ref          VARCHAR(64)    NOT NULL DEFAULT '',
    idempotency_key       VARCHAR(128)   NOT NULL DEFAULT '',
    bind_idempotency_key  VARCHAR(128)   NOT NULL DEFAULT '',
    created_at            DATETIME(3)    NOT NULL,
    updated_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (lease_id),
    INDEX idx_del_state (state),
    INDEX idx_del_entry (entry_id, entry_gen),
    INDEX idx_del_user (user_id, chain),
    INDEX idx_del_idem (idempotency_key),
    INDEX idx_del_expire (state, issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='白名单条目生命周期租约；出金提交必须携带未过期 dest_entry_lease_id。';

CREATE TABLE IF NOT EXISTS dest_entry_drift_proofs (
    proof_id              VARCHAR(36)    NOT NULL,
    kind                  VARCHAR(32)    NOT NULL,
    lease_id              VARCHAR(36)    NOT NULL DEFAULT '',
    entry_id              VARCHAR(64)    NOT NULL DEFAULT '',
    user_id               VARCHAR(64)    NOT NULL DEFAULT '',
    chain                 VARCHAR(32)    NOT NULL DEFAULT '',
    address               VARCHAR(128)   NOT NULL DEFAULT '',
    bind_mode             VARCHAR(16)    NOT NULL DEFAULT '',
    live_mode             VARCHAR(16)    NOT NULL DEFAULT '',
    bind_gen              BIGINT         NOT NULL DEFAULT 0,
    live_gen              BIGINT         NOT NULL DEFAULT 0,
    evidence              TEXT           NOT NULL,
    detected_at           DATETIME(3)    NOT NULL,
    exported_at           DATETIME(3)    NULL,
    created_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (proof_id),
    INDEX idx_dedp_kind (kind),
    INDEX idx_dedp_lease (lease_id),
    INDEX idx_dedp_entry (entry_id),
    INDEX idx_dedp_detected (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='白名单条目漂移拒出金证明，只追加。';

CREATE TABLE IF NOT EXISTS dest_entry_audit (
    audit_id              VARCHAR(36)    NOT NULL,
    lease_id              VARCHAR(36)    NOT NULL DEFAULT '',
    entry_id              VARCHAR(64)    NOT NULL DEFAULT '',
    action                VARCHAR(32)    NOT NULL,
    actor                 VARCHAR(64)    NOT NULL DEFAULT '',
    detail                VARCHAR(512)   NOT NULL DEFAULT '',
    created_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (audit_id),
    INDEX idx_dea_lease (lease_id),
    INDEX idx_dea_entry (entry_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='白名单条目租约生命周期审计。';

CREATE TABLE IF NOT EXISTS dest_entry_generations (
    entry_id              VARCHAR(64)    NOT NULL,
    user_id               VARCHAR(64)    NOT NULL DEFAULT '',
    chain                 VARCHAR(32)    NOT NULL DEFAULT '',
    address               VARCHAR(128)   NOT NULL DEFAULT '',
    entry_gen             BIGINT         NOT NULL DEFAULT 1,
    mode                  VARCHAR(16)    NOT NULL DEFAULT '',
    bump_reason           VARCHAR(64)    NOT NULL DEFAULT '',
    bumped_at             DATETIME(3)    NOT NULL,
    bumped_by             VARCHAR(64)    NOT NULL DEFAULT '',
    PRIMARY KEY (entry_id),
    INDEX idx_deg_user (user_id, chain)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='白名单条目级世代；Temp/破窗/Revoke/Restore 未满抬升。';
