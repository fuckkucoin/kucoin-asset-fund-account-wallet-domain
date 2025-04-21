-- 出金分片腿 Advance 冻结与 Release 残释竞态收口引擎：数据库表定义。
--
-- 设计原则：
--   1. wdshard_leg_hold_gens：每条腿冻结共享一行，钉扎 plan_id / leg_id /
--      hold_id / TTL / generation；FSM：issued → held → released | expired。
--   2. wdshard_hold_proofs：双占、漏释、在播挡写、无世代、超额释放证明，只追加。
--   3. wdshard_half_leg_cases：冻成功未广播，或广播成功未对齐；open → resumed | abandoned。
--   4. wdshard_hold_gen_audit：世代生命周期审计。
--
-- 隔离：
--   - 不改写 r5 大额出金分片部分成交公式表；
--   - 不改写 r14 分片剩余释放会计表；
--   - 不改写 wdpartial remgate 核心表；
--   - 不改写 wdbatch residue_fence 主路径表。
--
-- 作者：Kinsley
-- 日期：2020.12.27

CREATE TABLE IF NOT EXISTS wdshard_leg_hold_gens (
    gen_id                VARCHAR(40)    NOT NULL,
    plan_id               VARCHAR(64)    NOT NULL,
    leg_id                VARCHAR(64)    NOT NULL,
    hold_id               VARCHAR(64)    NOT NULL DEFAULT '',
    idempotency_key       VARCHAR(128)   NOT NULL,
    user_id               VARCHAR(64)    NOT NULL DEFAULT '',
    asset                 VARCHAR(16)    NOT NULL DEFAULT '',
    amount                BIGINT         NOT NULL,
    hold_amt              BIGINT         NOT NULL DEFAULT 0,
    fingerprint           VARCHAR(64)    NOT NULL DEFAULT '',
    generation            BIGINT         NOT NULL DEFAULT 1,
    state                 VARCHAR(16)    NOT NULL DEFAULT 'issued'
                            COMMENT 'issued|held|released|expired',
    ttl_seconds           BIGINT         NOT NULL DEFAULT 300,
    issued_at             DATETIME(3)    NOT NULL,
    held_at               DATETIME(3)    NULL,
    released_at           DATETIME(3)    NULL,
    expired_at            DATETIME(3)    NULL,
    broadcast_id          VARCHAR(64)    NOT NULL DEFAULT '',
    live_hash             VARCHAR(64)    NOT NULL DEFAULT '',
    release_amt           BIGINT         NOT NULL DEFAULT 0,
    mutex_owner           VARCHAR(16)    NOT NULL DEFAULT '',
    advance_count         BIGINT         NOT NULL DEFAULT 0,
    release_count         BIGINT         NOT NULL DEFAULT 0,
    created_at            DATETIME(3)    NOT NULL,
    updated_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (gen_id),
    UNIQUE KEY uk_lhg_idem (idempotency_key),
    UNIQUE KEY uk_lhg_leg (plan_id, leg_id),
    INDEX idx_lhg_state (state),
    INDEX idx_lhg_hold (hold_id),
    INDEX idx_lhg_expire (state, issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='分片腿冻结世代；Advance 与 Release 必须共享 leg_hold_gen。';

CREATE TABLE IF NOT EXISTS wdshard_hold_proofs (
    proof_id              VARCHAR(40)    NOT NULL,
    kind                  VARCHAR(32)    NOT NULL,
    gen_id                VARCHAR(40)    NOT NULL DEFAULT '',
    plan_id               VARCHAR(64)    NOT NULL DEFAULT '',
    leg_id                VARCHAR(64)    NOT NULL DEFAULT '',
    hold_id               VARCHAR(64)    NOT NULL DEFAULT '',
    idempotency_key       VARCHAR(128)   NOT NULL DEFAULT '',
    amount                BIGINT         NOT NULL DEFAULT 0,
    bind_gen              BIGINT         NOT NULL DEFAULT 0,
    live_gen              BIGINT         NOT NULL DEFAULT 0,
    bind_hash             VARCHAR(64)    NOT NULL DEFAULT '',
    live_hash             VARCHAR(64)    NOT NULL DEFAULT '',
    digest                VARCHAR(64)    NOT NULL DEFAULT '',
    evidence              TEXT           NOT NULL,
    detected_at           DATETIME(3)    NOT NULL,
    exported_at           DATETIME(3)    NULL,
    created_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (proof_id),
    INDEX idx_shp_kind (kind),
    INDEX idx_shp_gen (gen_id),
    INDEX idx_shp_leg (leg_id),
    INDEX idx_shp_detected (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='分片腿双占/漏释/在播挡写证明，只追加。';

CREATE TABLE IF NOT EXISTS wdshard_half_leg_cases (
    case_id               VARCHAR(40)    NOT NULL,
    gen_id                VARCHAR(40)    NOT NULL,
    plan_id               VARCHAR(64)    NOT NULL,
    leg_id                VARCHAR(64)    NOT NULL,
    hold_id               VARCHAR(64)    NOT NULL DEFAULT '',
    kind                  VARCHAR(32)    NOT NULL,
    state                 VARCHAR(16)    NOT NULL DEFAULT 'open'
                            COMMENT 'open|resumed|abandoned',
    actor                 VARCHAR(64)    NOT NULL DEFAULT '',
    resume_mutex          TINYINT        NOT NULL DEFAULT 0,
    note                  VARCHAR(256)   NOT NULL DEFAULT '',
    created_at            DATETIME(3)    NOT NULL,
    updated_at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (case_id),
    INDEX idx_hlc_gen (gen_id),
    INDEX idx_hlc_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='半截 Advance：冻成功未广播或广播未对齐。';

CREATE TABLE IF NOT EXISTS wdshard_hold_gen_audit (
    audit_id              VARCHAR(40)    NOT NULL,
    gen_id                VARCHAR(40)    NOT NULL DEFAULT '',
    action                VARCHAR(32)    NOT NULL,
    actor                 VARCHAR(64)    NOT NULL DEFAULT '',
    from_state            VARCHAR(16)    NOT NULL DEFAULT '',
    to_state              VARCHAR(16)    NOT NULL DEFAULT '',
    note                  VARCHAR(256)   NOT NULL DEFAULT '',
    at                    DATETIME(3)    NOT NULL,
    PRIMARY KEY (audit_id),
    INDEX idx_lhga_gen (gen_id, at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='腿占世代审计。';
