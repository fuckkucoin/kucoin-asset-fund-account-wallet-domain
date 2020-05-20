-- invdrift：偏离批次、条目、封堵建议与证明表。
-- 不关联 aa_ledger_drift_block；使用独立表族，避免与 r6 leddiff 冲突。
--
-- 作者：Eco
-- 日期：2025.11.03

-- ---- drift_batches ----
CREATE TABLE IF NOT EXISTS drift_batches (
    id           VARCHAR(36)  NOT NULL,
    state        VARCHAR(16)  NOT NULL DEFAULT 'open',
    trigger_by   VARCHAR(64)  NOT NULL DEFAULT '',
    data_center  VARCHAR(32)  NOT NULL DEFAULT '',
    scan_at      DATETIME(3)  NOT NULL,
    graded_at    DATETIME(3)  NULL,
    sealed_at    DATETIME(3)  NULL,
    resolved_at  DATETIME(3)  NULL,
    crit_count   INT          NOT NULL DEFAULT 0,
    warn_count   INT          NOT NULL DEFAULT 0,
    info_count   INT          NOT NULL DEFAULT 0,
    created_at   DATETIME(3)  NOT NULL,
    updated_at   DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_drift_batches_state (state),
    INDEX idx_drift_batches_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- drift_items ----
CREATE TABLE IF NOT EXISTS drift_items (
    id           VARCHAR(36)   NOT NULL,
    batch_id     VARCHAR(36)   NOT NULL,
    asset        VARCHAR(16)   NOT NULL,
    data_center  VARCHAR(32)   NOT NULL DEFAULT '',
    ledger_hot   VARCHAR(40)   NOT NULL DEFAULT '0',
    ledger_cold  VARCHAR(40)   NOT NULL DEFAULT '0',
    ledger_warm  VARCHAR(40)   NOT NULL DEFAULT '0',
    cust_hot     VARCHAR(40)   NOT NULL DEFAULT '0',
    cust_cold    VARCHAR(40)   NOT NULL DEFAULT '0',
    inv_hot      VARCHAR(40)   NOT NULL DEFAULT '0',
    inv_cold     VARCHAR(40)   NOT NULL DEFAULT '0',
    delta_hot    VARCHAR(40)   NOT NULL DEFAULT '0',
    delta_cold   VARCHAR(40)   NOT NULL DEFAULT '0',
    delta_pct    VARCHAR(20)   NOT NULL DEFAULT '0',
    grade        VARCHAR(8)    NOT NULL DEFAULT 'info',
    graded_at    DATETIME(3)   NULL,
    resolved_at  DATETIME(3)   NULL,
    created_at   DATETIME(3)   NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_drift_items_batch (batch_id),
    INDEX idx_drift_items_asset (asset),
    INDEX idx_drift_items_grade (grade)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- seal_advices ----
CREATE TABLE IF NOT EXISTS seal_advices (
    id             VARCHAR(36)   NOT NULL,
    batch_id       VARCHAR(36)   NOT NULL,
    item_id        VARCHAR(36)   NOT NULL,
    asset          VARCHAR(16)   NOT NULL,
    kind           VARCHAR(32)   NOT NULL,
    state          VARCHAR(16)   NOT NULL DEFAULT 'proposed',
    grade          VARCHAR(8)    NOT NULL DEFAULT 'crit',
    proposed_at    DATETIME(3)   NOT NULL,
    acked_at       DATETIME(3)   NULL,
    acked_by       VARCHAR(64)   NOT NULL DEFAULT '',
    expired_at     DATETIME(3)   NULL,
    expire_reason  VARCHAR(128)  NOT NULL DEFAULT '',
    ttl_seconds    INT           NOT NULL DEFAULT 1800,
    note           TEXT,
    created_at     DATETIME(3)   NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_seal_advices_batch (batch_id),
    INDEX idx_seal_advices_state (state),
    INDEX idx_seal_advices_asset (asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---- drift_proofs ----
CREATE TABLE IF NOT EXISTS drift_proofs (
    id               VARCHAR(36)   NOT NULL,
    batch_id         VARCHAR(36)   NOT NULL,
    asset            VARCHAR(16)   NOT NULL,
    ledger_digest    VARCHAR(64)   NOT NULL,
    custody_digest   VARCHAR(64)   NOT NULL,
    hotinv_digest    VARCHAR(64)   NOT NULL,
    ledger_snap      MEDIUMTEXT    NOT NULL,
    custody_snap     MEDIUMTEXT    NOT NULL,
    hotinv_snap      MEDIUMTEXT    NOT NULL,
    delta_hot        VARCHAR(40)   NOT NULL DEFAULT '0',
    delta_cold       VARCHAR(40)   NOT NULL DEFAULT '0',
    composite_hash   VARCHAR(64)   NOT NULL,
    grade            VARCHAR(8)    NOT NULL DEFAULT 'info',
    recorded_at      DATETIME(3)   NOT NULL,
    created_at       DATETIME(3)   NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_drift_proofs_batch (batch_id),
    INDEX idx_drift_proofs_asset (asset),
    UNIQUE INDEX uq_drift_proofs_batch_asset (batch_id, asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
