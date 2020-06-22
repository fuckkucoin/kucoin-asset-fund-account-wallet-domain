-- 出金地址白名单冷却生命周期表族。
-- 包含：dest_entry_lives、cooldown_windows、inflight_refs、bypass_proofs、lifecycle_audit。
-- 不含风险簇图谱或动态限额占用表（与 r4 G 正交）。
--
-- 作者：Eco
-- 日期：2025.04.20

-- ---- dest_entry_lives ----
-- 白名单条目生命周期记录。状态机：added→cooling→active→revoking→revoked。
-- entry_life_id 全局唯一；(user_id, chain, address) 在 state != revoked 时唯一（应用层强制）。
CREATE TABLE IF NOT EXISTS dest_entry_lives (
    entry_life_id   VARCHAR(36)   NOT NULL,
    user_id         VARCHAR(64)   NOT NULL,
    chain           VARCHAR(16)   NOT NULL,
    address         VARCHAR(128)  NOT NULL,
    label           VARCHAR(128)  NOT NULL DEFAULT '',
    state           VARCHAR(16)   NOT NULL DEFAULT 'cooling',
    added_at        DATETIME(3)   NOT NULL,
    cooling_from    DATETIME(3)   NOT NULL,
    active_at       DATETIME(3)   NULL,
    revoking_at     DATETIME(3)   NULL,
    revoked_at      DATETIME(3)   NULL,
    generation      BIGINT        NOT NULL DEFAULT 0,
    created_at      DATETIME(3)   NOT NULL,
    updated_at      DATETIME(3)   NOT NULL,
    PRIMARY KEY (entry_life_id),
    KEY idx_del_user_state   (user_id, state, created_at),
    KEY idx_del_addr         (user_id, chain, address, state),
    KEY idx_del_cooling      (state, cooling_from),
    KEY idx_del_revoking     (state, revoking_at),
    KEY idx_del_updated      (updated_at)
);

-- ---- cooldown_windows ----
-- 冷却策略窗口（scope=global 为全局默认；scope=user_id 为用户覆盖）。
CREATE TABLE IF NOT EXISTS cooldown_windows (
    id          VARCHAR(36)  NOT NULL,
    scope       VARCHAR(64)  NOT NULL,
    seconds     BIGINT       NOT NULL DEFAULT 86400,
    created_at  DATETIME(3)  NOT NULL,
    updated_at  DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_cw_scope (scope),
    KEY idx_cw_updated (updated_at)
);

-- ---- inflight_refs ----
-- 出金在途引用冻结记录。released=0 表示活跃；released=1 表示已释放。
-- movement_id 唯一约束防止重复获取。
CREATE TABLE IF NOT EXISTS inflight_refs (
    ref_id          VARCHAR(36)   NOT NULL,
    entry_life_id   VARCHAR(36)   NOT NULL,
    user_id         VARCHAR(64)   NOT NULL,
    chain           VARCHAR(16)   NOT NULL,
    address         VARCHAR(128)  NOT NULL,
    movement_id     VARCHAR(128)  NOT NULL,
    acquired_at     DATETIME(3)   NOT NULL,
    released        TINYINT       NOT NULL DEFAULT 0,
    released_at     DATETIME(3)   NULL,
    PRIMARY KEY (ref_id),
    UNIQUE KEY uk_ir_movement   (movement_id),
    KEY idx_ir_entry_active     (entry_life_id, released),
    KEY idx_ir_user             (user_id, released),
    KEY idx_ir_acquired         (acquired_at)
);

-- ---- bypass_proofs ----
-- 冷却绕过证明（append-only）。blocked=0 表示待巡检；blocked=1 表示已挡写。
CREATE TABLE IF NOT EXISTS bypass_proofs (
    proof_id        VARCHAR(36)   NOT NULL,
    entry_life_id   VARCHAR(36)   NOT NULL,
    user_id         VARCHAR(64)   NOT NULL,
    chain           VARCHAR(16)   NOT NULL,
    address         VARCHAR(128)  NOT NULL,
    reason          VARCHAR(64)   NOT NULL DEFAULT '',
    clock_drift_ms  BIGINT        NOT NULL DEFAULT 0,
    blocked         TINYINT       NOT NULL DEFAULT 0,
    blocked_by      VARCHAR(64)   NOT NULL DEFAULT '',
    detected_at     DATETIME(3)   NOT NULL,
    blocked_at      DATETIME(3)   NULL,
    PRIMARY KEY (proof_id),
    KEY idx_bp_entry        (entry_life_id, detected_at),
    KEY idx_bp_unblocked    (blocked, detected_at),
    KEY idx_bp_user         (user_id, detected_at)
);

-- ---- lifecycle_audit ----
-- 条目生命周期全轨迹审计（append-only）。
-- from_state 为空串表示初始化事件。
CREATE TABLE IF NOT EXISTS lifecycle_audit (
    audit_id        VARCHAR(36)   NOT NULL,
    entry_life_id   VARCHAR(36)   NOT NULL,
    user_id         VARCHAR(64)   NOT NULL,
    chain           VARCHAR(16)   NOT NULL,
    address         VARCHAR(128)  NOT NULL,
    actor           VARCHAR(64)   NOT NULL DEFAULT '',
    event           VARCHAR(64)   NOT NULL DEFAULT '',
    from_state      VARCHAR(16)   NOT NULL DEFAULT '',
    to_state        VARCHAR(16)   NOT NULL DEFAULT '',
    note            VARCHAR(255)  NOT NULL DEFAULT '',
    generation      BIGINT        NOT NULL DEFAULT 0,
    at              DATETIME(3)   NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_la_entry    (entry_life_id, at),
    KEY idx_la_user     (user_id, at),
    KEY idx_la_event    (event, at)
);

-- 初始化全局默认冷却策略 24h（86400s）。可通过 API 覆盖。
INSERT IGNORE INTO cooldown_windows (id, scope, seconds, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'global', 86400, NOW(3), NOW(3));
// temporary guard removed after the production check
