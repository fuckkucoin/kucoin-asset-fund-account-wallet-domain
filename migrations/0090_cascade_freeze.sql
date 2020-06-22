-- 战争室应急级联冻结协议：数据表迁移
-- 包含：cascade_plans, freeze_waves, wave_user_entries,
--       unfreeze_phases, coverage_proofs, rollback_legs
-- 作者：Axel
--
-- 设计约束：
--   - 所有表使用 UUID 主键，避免全局序列依赖。
--   - target_users / user_set / delta_users 以 JSON 数组文本存储，避免跨表大量外键。
--   - wave_user_entries 单独表，支持幂等重试记录（attempt_n 递增）。
--   - coverage_proofs 只追加；历史 proof 永久保留供审计。
--   - rollback_legs 一用户一腿；同 plan+user+reason 唯一索引防重复提交。

-- ─── cascade_plans ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS cascade_plans (
    id            VARCHAR(36)    NOT NULL,
    incident_ref  VARCHAR(128)   NOT NULL,
    scope         VARCHAR(16)    NOT NULL,        -- global/asset/cohort/user_set
    asset         VARCHAR(16)    NOT NULL DEFAULT '',
    cohort_tag    VARCHAR(128)   NOT NULL DEFAULT '',
    user_set      TEXT           NOT NULL DEFAULT '[]',  -- JSON 数组
    status        VARCHAR(16)    NOT NULL,        -- draft/armed/executing/sealed/aborted
    opened_by     VARCHAR(64)    NOT NULL,
    armed_by      VARCHAR(64)    NOT NULL DEFAULT '',
    aborted_by    VARCHAR(64)    NOT NULL DEFAULT '',
    abort_reason  VARCHAR(256)   NOT NULL DEFAULT '',
    dual_req_id   VARCHAR(36)    NOT NULL DEFAULT '',
    expires_at    DATETIME(3)    NOT NULL,
    sealed_at     DATETIME(3)             DEFAULT NULL,
    created_at    DATETIME(3)    NOT NULL,
    updated_at    DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_cascade_plans_status (status),
    INDEX idx_cascade_plans_incident (incident_ref),
    INDEX idx_cascade_plans_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='事故级联冻结计划';

-- ─── freeze_waves ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS freeze_waves (
    id            VARCHAR(36)    NOT NULL,
    plan_id       VARCHAR(36)    NOT NULL,
    seq_no        INT            NOT NULL DEFAULT 1,    -- 波次序号，从 1 开始
    label         VARCHAR(128)   NOT NULL DEFAULT '',
    target_users  TEXT           NOT NULL DEFAULT '[]', -- JSON 数组
    status        VARCHAR(16)    NOT NULL,              -- pending/running/done/failed/skipped
    frozen_count  INT            NOT NULL DEFAULT 0,
    failed_count  INT            NOT NULL DEFAULT 0,
    retry_count   INT            NOT NULL DEFAULT 0,
    started_at    DATETIME(3)             DEFAULT NULL,
    finished_at   DATETIME(3)             DEFAULT NULL,
    created_at    DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_freeze_waves_plan (plan_id),
    INDEX idx_freeze_waves_plan_seq (plan_id, seq_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='级联冻结执行波次';

-- ─── wave_user_entries ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wave_user_entries (
    id            VARCHAR(36)    NOT NULL,
    wave_id       VARCHAR(36)    NOT NULL,
    plan_id       VARCHAR(36)    NOT NULL,
    user_id       VARCHAR(64)    NOT NULL,
    result        VARCHAR(16)    NOT NULL,  -- ok/skipped/failed
    err_msg       VARCHAR(512)   NOT NULL DEFAULT '',
    attempt_n     INT            NOT NULL DEFAULT 1,
    at            DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_wave_entries_wave (wave_id),
    INDEX idx_wave_entries_plan (plan_id),
    INDEX idx_wave_entries_user (user_id),
    INDEX idx_wave_entries_wave_user (wave_id, user_id, attempt_n)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='波次内单用户冻结执行明细（支持幂等重试记录）';

-- ─── unfreeze_phases ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS unfreeze_phases (
    id              VARCHAR(36)    NOT NULL,
    plan_id         VARCHAR(36)    NOT NULL,
    kind            VARCHAR(16)    NOT NULL,   -- probe/partial/full/closed
    status          VARCHAR(16)    NOT NULL,   -- pending/running/passed/failed
    dual_req_id     VARCHAR(36)    NOT NULL DEFAULT '',
    initiated_by    VARCHAR(64)    NOT NULL,
    approved_by     VARCHAR(64)    NOT NULL DEFAULT '',
    integrity_note  VARCHAR(1024)  NOT NULL DEFAULT '',
    unfrozen_count  INT            NOT NULL DEFAULT 0,
    failed_count    INT            NOT NULL DEFAULT 0,
    started_at      DATETIME(3)             DEFAULT NULL,
    finished_at     DATETIME(3)             DEFAULT NULL,
    created_at      DATETIME(3)    NOT NULL,
    updated_at      DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_unfreeze_phases_plan (plan_id),
    INDEX idx_unfreeze_phases_plan_kind (plan_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='分阶段解冻记录（probe→partial→full）';

-- ─── coverage_proofs ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS coverage_proofs (
    id              VARCHAR(36)    NOT NULL,
    plan_id         VARCHAR(36)    NOT NULL,
    expected_count  INT            NOT NULL DEFAULT 0,
    actual_count    INT            NOT NULL DEFAULT 0,
    delta_count     INT            NOT NULL DEFAULT 0,
    delta_users     TEXT           NOT NULL DEFAULT '[]',  -- JSON 数组
    verdict         VARCHAR(16)    NOT NULL,  -- passed/gap/pending
    checked_at      DATETIME(3)    NOT NULL,
    created_at      DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_coverage_proofs_plan (plan_id),
    INDEX idx_coverage_proofs_plan_checked (plan_id, checked_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='覆盖率完整性证明（应冻集合 vs 实冻集合）；只追加，永久保留';

-- ─── rollback_legs ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS rollback_legs (
    id                  VARCHAR(36)    NOT NULL,
    plan_id             VARCHAR(36)    NOT NULL,
    user_id             VARCHAR(64)    NOT NULL,
    reason              VARCHAR(32)    NOT NULL,   -- mis_freeze/expired_incident/scope_error
    status              VARCHAR(16)    NOT NULL,   -- pending/executing/restored/failed
    dual_req_id         VARCHAR(36)    NOT NULL DEFAULT '',
    requested_by        VARCHAR(64)    NOT NULL,
    executed_by         VARCHAR(64)    NOT NULL DEFAULT '',
    availability_note   VARCHAR(1024)  NOT NULL DEFAULT '',
    has_live_hash       TINYINT(1)     NOT NULL DEFAULT 0,
    frozen_by_plan      TINYINT(1)     NOT NULL DEFAULT 1,
    amount              VARCHAR(48)    NOT NULL DEFAULT '0',
    created_at          DATETIME(3)    NOT NULL,
    updated_at          DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uniq_rollback_leg_plan_user_reason (plan_id, user_id, reason),
    INDEX idx_rollback_legs_plan (plan_id),
    INDEX idx_rollback_legs_user (user_id),
    INDEX idx_rollback_legs_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='误冻或过期事故回滚腿；执行后恢复用户可用视图解释';
