-- 功能：气隙密封导入世代租约与相位互斥表结构。
-- 边界：
--   airgap_import_leases   — ImportSealed 开立的独占世代租约（session_id ⊕ import_gen ⊕ blob_digest）。
--   airgap_blob_digests    — 成功导入的密封 blob SHA-256 摘要（append-only，唯一索引防重放）。
--   airgap_phase_conflicts — Import 与 Abort/Close/Export 并发冲突证明（append-only）。
--   airgap_quorum_binds    — 导入时法定人数绑定世代记录（append-only）。
-- 不变量：
--   airgap_import_leases 无 DELETE；状态推进只通过 UPDATE SET state。
--   airgap_blob_digests 无 UPDATE；INSERT 唯一冲突即重放。
--   airgap_phase_conflicts 仅通过 UPDATE SET state 推进；只追加。
--   airgap_quorum_binds 无 UPDATE/DELETE；只追加证明。
--   同 session_id 的 airgap_import_leases state NOT IN ('imported','aborted','expired') 最多一条（业务层保证）。
--
-- 作者：Eco
-- 日期：2025.11.04

-- ── airgap_import_leases ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS airgap_import_leases (
    lease_id      VARCHAR(64)  NOT NULL,
    session_id    VARCHAR(64)  NOT NULL             COMMENT '关联气隙仪式 Session ID',
    import_gen    VARCHAR(128) NOT NULL DEFAULT ''  COMMENT '导入世代令牌（air_import_gen）',
    blob_digest   VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '密封 blob SHA-256 hex',
    actor         VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '触发导入的操作人',

    state         VARCHAR(16)  NOT NULL DEFAULT 'open'
                      COMMENT '租约状态：open/importing/imported/aborted/expired',

    expires_at    DATETIME(3)  NOT NULL             COMMENT '租约到期时间（UTC）',
    imported_at   DATETIME(3)  NULL                 COMMENT '导入成功时间',
    aborted_at    DATETIME(3)  NULL                 COMMENT '中止/过期时间',
    abort_reason  VARCHAR(255) NOT NULL DEFAULT ''  COMMENT '中止原因',

    created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (lease_id),
    INDEX idx_session_state (session_id, state),
    INDEX idx_expires_state (expires_at, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='气隙密封导入世代租约';

-- ── airgap_blob_digests ─────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS airgap_blob_digests (
    digest_id    VARCHAR(64)  NOT NULL,
    digest_hex   VARCHAR(64)  NOT NULL             COMMENT '密封 blob SHA-256 hex（全局唯一，防重放）',
    session_id   VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '关联 Session ID',
    lease_id     VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '关联租约 ID',
    imported_by  VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '导入操作人',
    imported_at  DATETIME(3)  NOT NULL             COMMENT '导入成功时间',

    PRIMARY KEY (digest_id),
    UNIQUE KEY uq_digest_hex (digest_hex),
    INDEX idx_session (session_id),
    INDEX idx_lease (lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='密封 blob 摘要（重放防护，append-only）';

-- ── airgap_phase_conflicts ──────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS airgap_phase_conflicts (
    conflict_id       VARCHAR(64)  NOT NULL,
    session_id        VARCHAR(64)  NOT NULL             COMMENT '关联 Session ID',
    conflict_kind     VARCHAR(32)  NOT NULL DEFAULT ''
                          COMMENT '冲突类型：import_vs_abort/import_vs_close/import_vs_export',
    state             VARCHAR(16)  NOT NULL DEFAULT 'open'
                          COMMENT '冲突状态：open/proven/closed',

    winner_op         VARCHAR(32)  NOT NULL DEFAULT ''  COMMENT '胜者操作名',
    winner_actor      VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '胜者操作人',
    loser_op          VARCHAR(32)  NOT NULL DEFAULT ''  COMMENT '败者操作名',
    loser_actor       VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '败者操作人',
    phase_at_conflict VARCHAR(32)  NOT NULL DEFAULT ''  COMMENT '冲突发生时 Session 相位',

    note          VARCHAR(512) NOT NULL DEFAULT '',
    proven_at     DATETIME(3)  NULL                     COMMENT '升级为 proven 的时间',
    closed_at     DATETIME(3)  NULL                     COMMENT '确认关闭时间',

    created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (conflict_id),
    INDEX idx_session_state (session_id, state),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='气隙相位冲突证明（append-only）';

-- ── airgap_quorum_binds ─────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS airgap_quorum_binds (
    bind_id       VARCHAR(64)  NOT NULL,
    session_id    VARCHAR(64)  NOT NULL             COMMENT '关联 Session ID',
    lease_id      VARCHAR(64)  NOT NULL             COMMENT '关联租约 ID',
    bind_gen      VARCHAR(128) NOT NULL DEFAULT ''  COMMENT '绑定世代（同 air_import_gen）',

    threshold     INT          NOT NULL DEFAULT 0   COMMENT 'Session Threshold 快照',
    actual_quorum INT          NOT NULL DEFAULT 0   COMMENT '实际不同签名人数',
    quorum_passed TINYINT(1)   NOT NULL DEFAULT 0   COMMENT '1=通过，0=不通过',

    bound_by      VARCHAR(64)  NOT NULL DEFAULT ''  COMMENT '绑定操作人',
    bound_at      DATETIME(3)  NOT NULL,
    created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (bind_id),
    INDEX idx_lease (lease_id),
    INDEX idx_session (session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='导入法定人数绑定世代（append-only）';
