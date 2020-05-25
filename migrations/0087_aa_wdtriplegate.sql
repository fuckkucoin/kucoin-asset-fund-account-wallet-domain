-- 功能：三联闸世代租约与漂移案件表结构。
-- 边界：
--   wd_triple_leases      — Preflight/CheckTriple 成功后签发的三联租约（ticket_clear ⊕ kyt_pin ⊕ list_gen ⊕ policy_ver）。
--   wd_preflight_binds    — 广播前世代复验绑定记录（append-only）。
--   wd_triple_drift_cases — 三联漂移案件（Detector 产出；append-only）。
-- 不变量：
--   wd_preflight_binds 无 UPDATE；写入后只读（append-only）。
--   wd_triple_drift_cases 仅通过 UPDATE SET state 推进状态，不增删字段。
--   同 movement_id 的 wd_triple_leases state IN ('issued','bound') 最多一条（业务层保证）。
--
-- 作者：Eco
-- 日期：2025.11.28

-- ── wd_triple_leases ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wd_triple_leases (
    triple_lease_id  VARCHAR(64)      NOT NULL,
    movement_id      VARCHAR(128)     NOT NULL,
    user_id          VARCHAR(64)      NOT NULL,
    chain            VARCHAR(32)      NOT NULL,
    dest             VARCHAR(256)     NOT NULL,

    -- 三联快照
    ticket_clear     TINYINT(1)       NOT NULL DEFAULT 0  COMMENT '签发时无未结异常单（1=true）',
    kyt_pin          VARCHAR(16)      NOT NULL DEFAULT ''  COMMENT 'KYT 决策快照（allow）',
    list_gen         BIGINT           NOT NULL DEFAULT 0   COMMENT 'destallow 白名单世代快照',
    policy_ver       INT              NOT NULL DEFAULT 0   COMMENT 'policypack 策略版本快照',

    state            VARCHAR(16)      NOT NULL DEFAULT 'issued'
                         COMMENT '租约状态：issued/bound/consumed/expired/blocked',
    issued_at        DATETIME(3)      NOT NULL,
    expires_at       DATETIME(3)      NOT NULL             COMMENT 'TTL 截止；过期后需重检',
    bound_at         DATETIME(3)      NULL,
    consumed_at      DATETIME(3)      NULL,
    expired_at       DATETIME(3)      NULL,
    blocked_at       DATETIME(3)      NULL,
    drift_case_id    VARCHAR(64)      NULL                 COMMENT '关联漂移案件（Blocked 时写入）',
    block_reason     VARCHAR(512)     NULL,
    issued_by        VARCHAR(128)     NOT NULL DEFAULT '',
    created_at       DATETIME(3)      NOT NULL,
    updated_at       DATETIME(3)      NOT NULL,

    PRIMARY KEY (triple_lease_id),
    KEY idx_wd_tl_movement_state (movement_id, state),
    KEY idx_wd_tl_expires_state  (expires_at, state),
    KEY idx_wd_tl_created        (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='出金三联闸世代租约（ticket_clear⊕kyt_pin⊕list_gen⊕policy_ver）';

-- ── wd_preflight_binds ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wd_preflight_binds (
    bind_id              VARCHAR(64)      NOT NULL,
    movement_id          VARCHAR(128)     NOT NULL,
    triple_lease_id      VARCHAR(64)      NOT NULL,

    list_gen_at_bind     BIGINT           NOT NULL DEFAULT 0   COMMENT '复验时读取的 destallow 世代',
    policy_ver_at_bind   INT              NOT NULL DEFAULT 0   COMMENT '复验时读取的策略版本',

    broadcast_allowed    TINYINT(1)       NOT NULL DEFAULT 0   COMMENT '是否允许广播（1=true）',
    refuse_reason        VARCHAR(512)     NULL,
    drift_detected       TINYINT(1)       NOT NULL DEFAULT 0   COMMENT '是否检测到漂移（1=true）',

    bound_at             DATETIME(3)      NOT NULL,
    created_at           DATETIME(3)      NOT NULL,

    PRIMARY KEY (bind_id),
    UNIQUE KEY uq_wd_pb_movement (movement_id),
    KEY idx_wd_pb_lease (triple_lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='广播前世代复验绑定记录（append-only）';

-- ── wd_triple_drift_cases ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wd_triple_drift_cases (
    drift_case_id        VARCHAR(64)      NOT NULL,
    movement_id          VARCHAR(128)     NOT NULL,
    triple_lease_id      VARCHAR(64)      NOT NULL,
    drift_kind           VARCHAR(16)      NOT NULL
                             COMMENT '漂移类型：list_gen/policy_ver/kyt_pin',

    -- 世代/版本核对
    lease_list_gen       BIGINT           NOT NULL DEFAULT 0,
    current_list_gen     BIGINT           NOT NULL DEFAULT 0,
    lease_policy_ver     INT              NOT NULL DEFAULT 0,
    current_policy_ver   INT              NOT NULL DEFAULT 0,
    lease_kyt_pin        VARCHAR(16)      NOT NULL DEFAULT '',
    current_kyt_pin      VARCHAR(16)      NOT NULL DEFAULT '',

    state                VARCHAR(16)      NOT NULL DEFAULT 'open'
                             COMMENT '案件状态：open/proven/closed',
    proof_checksum       VARCHAR(128)     NULL      COMMENT 'SHA-256 证明摘要',
    confirmed_by         VARCHAR(128)     NULL,
    co_confirmed_by      VARCHAR(128)     NULL,
    close_note           VARCHAR(512)     NULL,

    detected_at          DATETIME(3)      NOT NULL,
    proven_at            DATETIME(3)      NULL,
    closed_at            DATETIME(3)      NULL,
    created_at           DATETIME(3)      NOT NULL,
    updated_at           DATETIME(3)      NOT NULL,

    PRIMARY KEY (drift_case_id),
    KEY idx_wd_tdc_movement   (movement_id),
    KEY idx_wd_tdc_lease      (triple_lease_id),
    KEY idx_wd_tdc_state      (state),
    KEY idx_wd_tdc_detected   (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='三联漂移案件（list_gen/policy_ver/kyt_pin 漂移证明仓；append-only）';
