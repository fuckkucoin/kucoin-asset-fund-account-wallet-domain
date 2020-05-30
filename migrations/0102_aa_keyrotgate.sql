-- 功能：HSM 密钥槽轮换世代租约与在途签名互斥表结构。
-- 边界：
--   keycer_rotate_leases   — OpenRotate 后签发的轮换世代租约（source_fp ⊕ dst_fp ⊕ rotate_gen）。
--   keycer_sign_binds      — 在途签名/解锁操作对源槽指纹的绑定记录（append-only 释放后不删）。
--   keycer_rot_drift_cases — 指纹漂移案件（旧指纹拒签时产出；append-only 证明仓）。
--   keycer_fp_proofs       — 轮换完成后的指纹前后证明（append-only）。
-- 不变量：
--   keycer_sign_binds 无 DELETE；状态推进只通过 UPDATE SET state。
--   keycer_rot_drift_cases 仅通过 UPDATE SET state 推进；proof_checksum 唯一。
--   keycer_fp_proofs 无 UPDATE；写入后只读（append-only）。
--   同 ticket_id 的 keycer_rotate_leases state NOT IN ('done','blocked') 最多一条（业务层保证）。
--
-- 作者：Eco
-- 日期：2020.12.13

-- ── keycer_rotate_leases ─────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS keycer_rotate_leases (
    rotate_lease_id  VARCHAR(64)      NOT NULL,
    ticket_id        VARCHAR(64)      NOT NULL             COMMENT 'keycer rotate 工单 ID',
    src_slot_id      VARCHAR(64)      NOT NULL             COMMENT '源槽 ID',
    dst_slot_id      VARCHAR(64)      NOT NULL             COMMENT '目标槽 ID',

    -- 指纹快照（签发时锁定）
    src_fingerprint  VARCHAR(128)     NOT NULL DEFAULT ''  COMMENT '源槽指纹快照',
    dst_fingerprint  VARCHAR(128)     NOT NULL DEFAULT ''  COMMENT '目标槽指纹快照（空=dst 为 Empty）',
    rotate_gen       BIGINT           NOT NULL DEFAULT 0   COMMENT '本次轮换世代号（单调递增）',

    state            VARCHAR(16)      NOT NULL DEFAULT 'open'
                         COMMENT '租约状态：open/approved/executing/done/blocked',
    opened_by        VARCHAR(128)     NOT NULL DEFAULT '',
    approved_by      VARCHAR(128)     NULL,
    executed_by      VARCHAR(128)     NULL,
    note             VARCHAR(512)     NOT NULL DEFAULT '',

    opened_at        DATETIME(3)      NOT NULL,
    expires_at       DATETIME(3)      NOT NULL             COMMENT 'TTL 截止；超期后需重新 OpenRotate',
    approved_at      DATETIME(3)      NULL,
    executing_at     DATETIME(3)      NULL,
    done_at          DATETIME(3)      NULL,
    blocked_at       DATETIME(3)      NULL,

    block_reason     VARCHAR(512)     NULL,
    proof_id         VARCHAR(64)      NULL                 COMMENT '关联 FingerprintProof（Done 时写入）',

    created_at       DATETIME(3)      NOT NULL,
    updated_at       DATETIME(3)      NOT NULL,

    PRIMARY KEY (rotate_lease_id),
    UNIQUE KEY  uq_krl_ticket      (ticket_id),
    KEY         idx_krl_src_state  (src_slot_id, state),
    KEY         idx_krl_expires    (expires_at, state),
    KEY         idx_krl_created    (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='HSM 密钥槽轮换世代租约（source_fp⊕dst_fp⊕rotate_gen）';

-- ── keycer_sign_binds ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS keycer_sign_binds (
    sign_bind_id     VARCHAR(64)      NOT NULL,
    src_slot_id      VARCHAR(64)      NOT NULL             COMMENT '绑定的源槽 ID',
    fingerprint      VARCHAR(128)     NOT NULL DEFAULT ''  COMMENT '绑定时的槽指纹',
    operation_id     VARCHAR(128)     NOT NULL             COMMENT '签名/解锁操作外部 ID',
    operation_kind   VARCHAR(16)      NOT NULL DEFAULT ''  COMMENT '"sign" 或 "unlock"',
    bound_by         VARCHAR(128)     NOT NULL DEFAULT ''  COMMENT '发起操作的 Actor',

    state            VARCHAR(16)      NOT NULL DEFAULT 'active'
                         COMMENT '绑定状态：active/released/expired/rejected',
    bound_at         DATETIME(3)      NOT NULL,
    expires_at       DATETIME(3)      NOT NULL             COMMENT 'TTL 截止',

    released_at      DATETIME(3)      NULL,
    expired_at       DATETIME(3)      NULL,
    rejected_at      DATETIME(3)      NULL,
    drift_case_id    VARCHAR(64)      NULL                 COMMENT '被拒绝时关联漂移案件',

    created_at       DATETIME(3)      NOT NULL,
    updated_at       DATETIME(3)      NOT NULL,

    PRIMARY KEY (sign_bind_id),
    UNIQUE KEY  uq_ksb_operation   (operation_id),
    KEY         idx_ksb_src_state  (src_slot_id, state),
    KEY         idx_ksb_expires    (expires_at, state),
    KEY         idx_ksb_created    (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='在途签名/解锁操作对源槽指纹的绑定记录（append-only）';

-- ── keycer_rot_drift_cases ───────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS keycer_rot_drift_cases (
    drift_case_id    VARCHAR(64)      NOT NULL,
    src_slot_id      VARCHAR(64)      NOT NULL             COMMENT '已退役的源槽',
    old_fingerprint  VARCHAR(128)     NOT NULL DEFAULT ''  COMMENT '旧指纹（已 Retired 槽）',
    new_fingerprint  VARCHAR(128)     NOT NULL DEFAULT ''  COMMENT '新槽指纹（可为空）',
    sign_bind_id     VARCHAR(64)      NOT NULL             COMMENT '触发漂移的 SignBind',
    rotate_lease_id  VARCHAR(64)      NULL                 COMMENT '关联的轮换租约（可为空）',

    state            VARCHAR(16)      NOT NULL DEFAULT 'open'
                         COMMENT '漂移案件状态：open/proven/closed',

    drift_detected_at DATETIME(3)     NOT NULL,
    proven_at         DATETIME(3)     NULL,
    closed_at         DATETIME(3)     NULL,

    proof_checksum   VARCHAR(64)      NULL                 COMMENT '双人确认后写入；唯一',
    confirmed_by     VARCHAR(128)     NULL,
    co_confirmed_by  VARCHAR(128)     NULL,
    note             VARCHAR(512)     NOT NULL DEFAULT '',

    created_at       DATETIME(3)      NOT NULL,
    updated_at       DATETIME(3)      NOT NULL,

    PRIMARY KEY (drift_case_id),
    UNIQUE KEY  uq_krdc_proof      (proof_checksum),
    KEY         idx_krdc_src_state (src_slot_id, state),
    KEY         idx_krdc_bind      (sign_bind_id),
    KEY         idx_krdc_created   (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='指纹漂移案件（旧指纹拒签时产出；append-only 证明仓）';

-- ── keycer_fp_proofs ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS keycer_fp_proofs (
    proof_id           VARCHAR(64)    NOT NULL,
    rotate_lease_id    VARCHAR(64)    NOT NULL             COMMENT '关联轮换租约',
    ticket_id          VARCHAR(64)    NOT NULL             COMMENT 'keycer rotate 工单',
    src_slot_id        VARCHAR(64)    NOT NULL,
    dst_slot_id        VARCHAR(64)    NOT NULL,

    before_fingerprint VARCHAR(128)   NOT NULL DEFAULT ''  COMMENT '源槽退役前指纹',
    after_fingerprint  VARCHAR(128)   NOT NULL DEFAULT ''  COMMENT '目标槽加载后指纹',
    rotate_gen         BIGINT         NOT NULL DEFAULT 0   COMMENT '轮换世代号',

    proof_checksum     VARCHAR(64)    NOT NULL             COMMENT 'SHA-256(before||after||gen||ticket)',
    issued_by          VARCHAR(128)   NOT NULL DEFAULT '',
    issued_at          DATETIME(3)    NOT NULL,
    created_at         DATETIME(3)    NOT NULL,

    PRIMARY KEY (proof_id),
    UNIQUE KEY  uq_kfp_lease       (rotate_lease_id),
    UNIQUE KEY  uq_kfp_checksum    (proof_checksum),
    KEY         idx_kfp_src_slot   (src_slot_id, issued_at),
    KEY         idx_kfp_created    (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='HSM 轮换完成后的指纹前后证明（append-only）';
