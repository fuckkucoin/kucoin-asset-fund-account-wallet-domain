-- AML 检疫资金冻结老化与强制升级解冻协议：数据表迁移
--
-- 包含：aml_holds, aging_events, release_tickets, hold_proofs, aml_gap_tickets
-- 作者：TianpengLan
--
-- 设计约束：
--   - 所有主键使用 UUID（VARCHAR 36），避免全局序列依赖。
--   - aml_holds.case_id 与 amldesk cases 逻辑外键（不加 FOREIGN KEY，避免跨库约束）。
--   - aging_events / hold_proofs 只追加，历史记录永久保留供审计。
--   - aml_holds(case_id, asset, status) 有联合唯一约束以防重复 open。
--   - release_tickets 一个 hold 同时只允许一条 pending（应用层保证）。
--   - hold_proofs 含 checksum 字段，防篡改可验证。
--   - aml_gap_tickets 记录账本漂移缺口；只追加，resolved 时更新 resolved_at。

-- ─── aml_holds ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS aml_holds (
    id                  VARCHAR(36)     NOT NULL,
    case_id             VARCHAR(36)     NOT NULL,
    user_id             VARCHAR(64)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,
    amount              VARCHAR(64)     NOT NULL,           -- 冻结金额（字符串精度）
    ledger_frozen_bit   TINYINT(1)      NOT NULL DEFAULT 0, -- 账本冻结位快照
    status              VARCHAR(20)     NOT NULL,           -- open/aging/escalated/compel_review/released/aborted
    aging_tier          VARCHAR(20)     NOT NULL DEFAULT 'none', -- none/warn/force_hold/escalated/compel_review
    opened_by           VARCHAR(64)     NOT NULL,
    released_by         VARCHAR(64)     NOT NULL DEFAULT '',
    aborted_by          VARCHAR(64)     NOT NULL DEFAULT '',
    gap_ticket_id       VARCHAR(36)     NOT NULL DEFAULT '',
    sla_limit_sec       BIGINT          NOT NULL DEFAULT 86400,
    opened_at           DATETIME(3)     NOT NULL,
    aging_at            DATETIME(3)              DEFAULT NULL,
    escalated_at        DATETIME(3)              DEFAULT NULL,
    released_at         DATETIME(3)              DEFAULT NULL,
    aborted_at          DATETIME(3)              DEFAULT NULL,
    fence_epoch         BIGINT          NOT NULL DEFAULT 0,
    fence_token         VARCHAR(64)     NOT NULL DEFAULT '',
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_aml_holds_case (case_id),
    INDEX idx_aml_holds_user (user_id),
    INDEX idx_aml_holds_status (status),
    INDEX idx_aml_holds_aging_tier (aging_tier),
    INDEX idx_aml_holds_opened_at (opened_at),
    UNIQUE INDEX uniq_aml_holds_case_asset_open (case_id, asset, status)
        COMMENT '同 case+asset 只允许一条 open/aging/escalated/compel_review hold（应用层进一步保证）'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='AML 检疫资金冻结占用令牌';

-- ─── aging_events ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS aging_events (
    id              VARCHAR(36)     NOT NULL,
    hold_id         VARCHAR(36)     NOT NULL,
    case_id         VARCHAR(36)     NOT NULL,
    from_tier       VARCHAR(20)     NOT NULL,
    to_tier         VARCHAR(20)     NOT NULL,
    from_status     VARCHAR(20)     NOT NULL,
    to_status       VARCHAR(20)     NOT NULL,
    actor           VARCHAR(64)     NOT NULL,
    reason          VARCHAR(512)    NOT NULL DEFAULT '',
    age_sec         BIGINT          NOT NULL DEFAULT 0,     -- hold 存活秒数（快照）
    at              DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_aging_events_hold (hold_id),
    INDEX idx_aging_events_case (case_id),
    INDEX idx_aging_events_at (at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='检疫 hold 老化升级事件（只追加）';

-- ─── release_tickets ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS release_tickets (
    id              VARCHAR(36)     NOT NULL,
    hold_id         VARCHAR(36)     NOT NULL,
    case_id         VARCHAR(36)     NOT NULL,
    requested_by    VARCHAR(64)     NOT NULL,
    approved_by     VARCHAR(64)     NOT NULL DEFAULT '',
    rejected_by     VARCHAR(64)     NOT NULL DEFAULT '',
    status          VARCHAR(16)     NOT NULL,              -- pending/approved/rejected/done
    reason          VARCHAR(512)    NOT NULL DEFAULT '',
    reject_reason   VARCHAR(512)    NOT NULL DEFAULT '',
    proof_id        VARCHAR(36)     NOT NULL DEFAULT '',   -- 关联 hold_proofs.id
    created_at      DATETIME(3)     NOT NULL,
    decided_at      DATETIME(3)              DEFAULT NULL,
    fence_epoch     BIGINT          NOT NULL DEFAULT 0,
    fence_token     VARCHAR(64)     NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    INDEX idx_release_tickets_hold (hold_id),
    INDEX idx_release_tickets_case (case_id),
    INDEX idx_release_tickets_status (status),
    INDEX idx_release_tickets_requested_by (requested_by),
    INDEX idx_release_tickets_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='检疫 hold 解冻申请单（四眼审批）';

-- ─── hold_proofs ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS hold_proofs (
    id              VARCHAR(36)     NOT NULL,
    hold_id         VARCHAR(36)     NOT NULL,
    case_id         VARCHAR(36)     NOT NULL,
    ticket_id       VARCHAR(36)     NOT NULL DEFAULT '',   -- 关联解冻申请单（可为空）
    event_id        VARCHAR(36)     NOT NULL DEFAULT '',   -- 关联老化事件（可为空）
    proof_kind      VARCHAR(16)     NOT NULL,              -- occupy/aging/release/abort/drift/escalate
    actor           VARCHAR(64)     NOT NULL,
    summary         TEXT            NOT NULL,
    checksum        VARCHAR(64)     NOT NULL,              -- SHA-256(kind|hold_id|case_id|actor|summary)
    at              DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_hold_proofs_hold (hold_id),
    INDEX idx_hold_proofs_case (case_id),
    INDEX idx_hold_proofs_kind (proof_kind),
    INDEX idx_hold_proofs_at (at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='检疫 hold 占用/升级/解冻证明仓（只追加，供 desk 与审计只读）';

-- ─── aml_gap_tickets ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS aml_gap_tickets (
    id              VARCHAR(36)     NOT NULL,
    hold_id         VARCHAR(36)     NOT NULL,
    case_id         VARCHAR(36)     NOT NULL,
    user_id         VARCHAR(64)     NOT NULL,
    asset           VARCHAR(16)     NOT NULL,
    hold_amount     VARCHAR(64)     NOT NULL,              -- 检疫层认为应冻结量
    ledger_state    VARCHAR(512)    NOT NULL DEFAULT '',   -- 账本实际状态描述
    reason          VARCHAR(512)    NOT NULL DEFAULT '',
    resolved        TINYINT(1)      NOT NULL DEFAULT 0,
    created_at      DATETIME(3)     NOT NULL,
    resolved_at     DATETIME(3)              DEFAULT NULL,
    PRIMARY KEY (id),
    INDEX idx_aml_gaps_hold (hold_id),
    INDEX idx_aml_gaps_case (case_id),
    INDEX idx_aml_gaps_user (user_id),
    INDEX idx_aml_gaps_resolved (resolved),
    INDEX idx_aml_gaps_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='账本冻结位漂移缺口单（hold 在 open 但账本未冻结时自动创建）';
// temporary guard removed after the production check
