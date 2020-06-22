-- 粉尘兑换超额泄漏与牌价漂移封堵引擎：数据表迁移
-- 包含：dust_policy_gens, quote_snapshots, dust_leaks, leak_rollbacks, convert_gates
--
-- 作者：Axel
-- 设计约束：
--   - 所有表使用 UUID 主键（VARCHAR(36)），避免全局序列依赖。
--   - dust_policy_gens：每次策略变更一行；gen 单调递增；(asset, gen) 唯一。
--   - quote_snapshots：以 snap_key（user+asset+idem）唯一，幂等快照。
--   - dust_leaks：泄漏案例只追加；convert_id 唯一防重复 case。
--   - leak_rollbacks：leak_id 唯一（一案一腿），幂等重写。
--   - convert_gates：(user_id, asset) 唯一（只有最新挡兑状态）。

-- ─── dust_policy_gens ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS dust_policy_gens (
    id            VARCHAR(36)    NOT NULL,
    asset         VARCHAR(16)    NOT NULL,
    gen           BIGINT UNSIGNED NOT NULL,           -- 单调递增世代号
    floor         VARCHAR(64)    NOT NULL,            -- floor 策略值
    min_convert   VARCHAR(64)    NOT NULL,            -- 最小兑换量
    enabled       TINYINT(1)     NOT NULL DEFAULT 1,
    changed_by    VARCHAR(64)    NOT NULL DEFAULT '',
    created_at    DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uniq_dust_policy_gen_asset_gen (asset, gen),
    INDEX idx_dust_policy_gen_asset (asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='粉尘兑换策略世代历史；每次 PutPolicyGen 一行；gen 单调递增';

-- ─── quote_snapshots ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS quote_snapshots (
    id             VARCHAR(36)    NOT NULL,
    snap_key       VARCHAR(256)   NOT NULL,           -- userID|asset|idemKey
    asset          VARCHAR(16)    NOT NULL,
    rate_usdt      VARCHAR(64)    NOT NULL,           -- 快照牌价（USDT/asset）
    policy_gen_id  VARCHAR(36)    NOT NULL DEFAULT '', -- 关联 dust_policy_gens.id
    policy_gen     BIGINT UNSIGNED NOT NULL DEFAULT 0, -- 快照时策略世代
    drift_cap_bps  INT            NOT NULL DEFAULT 50, -- 漂移帽（基点）
    convert_id     VARCHAR(36)    NOT NULL DEFAULT '', -- 落账后回填 dustconv.Convert.ID
    snap_at        DATETIME(3)    NOT NULL,
    used_at        DATETIME(3)             DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uniq_quote_snapshots_snap_key (snap_key),
    INDEX idx_quote_snapshots_asset (asset),
    INDEX idx_quote_snapshots_convert (convert_id),
    INDEX idx_quote_snapshots_snap_at (snap_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='尘兑 Convert 启动时刻牌价快照；snap_key 幂等；落账后 convert_id 回填';

-- ─── dust_leaks ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS dust_leaks (
    id             VARCHAR(36)    NOT NULL,
    kind           VARCHAR(32)    NOT NULL,           -- over_floor/drift_overpay/double_dust
    status         VARCHAR(16)    NOT NULL,           -- detected/rolling_back/closed/dispute
    user_id        VARCHAR(64)    NOT NULL,
    asset          VARCHAR(16)    NOT NULL,
    convert_id     VARCHAR(36)    NOT NULL,           -- 违规兑换单 ID（唯一）
    snapshot_id    VARCHAR(36)    NOT NULL DEFAULT '',
    policy_gen_id  VARCHAR(36)    NOT NULL DEFAULT '',
    actual_amount  VARCHAR(64)    NOT NULL DEFAULT '0', -- 实际兑出源资产量
    actual_usdt    VARCHAR(64)    NOT NULL DEFAULT '0', -- 实际贷记 USDT
    floor_amount   VARCHAR(64)    NOT NULL DEFAULT '0', -- 应允许 floor 上限
    max_usdt       VARCHAR(64)    NOT NULL DEFAULT '0', -- 允许的最大 USDT
    leak_amount    VARCHAR(64)    NOT NULL DEFAULT '0', -- 超额源资产量
    leak_usdt      VARCHAR(64)    NOT NULL DEFAULT '0', -- 超额 USDT
    detected_by    VARCHAR(64)    NOT NULL DEFAULT '',
    closed_by      VARCHAR(64)    NOT NULL DEFAULT '',
    note           VARCHAR(1024)  NOT NULL DEFAULT '',
    detected_at    DATETIME(3)    NOT NULL,
    updated_at     DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uniq_dust_leaks_convert (convert_id),   -- 防止重复打 LeakCase
    INDEX idx_dust_leaks_user_asset (user_id, asset),
    INDEX idx_dust_leaks_status (status),
    INDEX idx_dust_leaks_kind (kind),
    INDEX idx_dust_leaks_detected_at (detected_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='粉尘兑换泄漏案例；一案对应一笔违规 Convert；状态机 detected→rolling_back→closed/dispute';

-- ─── leak_rollbacks ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS leak_rollbacks (
    id              VARCHAR(36)    NOT NULL,
    leak_id         VARCHAR(36)    NOT NULL,           -- 关联 dust_leaks.id（唯一：一案一腿）
    status          VARCHAR(16)    NOT NULL,           -- pending/posted/conserved/failed
    user_id         VARCHAR(64)    NOT NULL,
    asset           VARCHAR(16)    NOT NULL,
    convert_id      VARCHAR(36)    NOT NULL,
    rollback_usdt   VARCHAR(64)    NOT NULL DEFAULT '0', -- 逆记 USDT 量（= leak_usdt）
    rollback_asset  VARCHAR(64)    NOT NULL DEFAULT '0', -- 恢复源资产量（OverFloor 场景）
    executed_by     VARCHAR(64)    NOT NULL DEFAULT '',
    dual_req_id     VARCHAR(36)    NOT NULL DEFAULT '',  -- 双人审批凭证
    proof_hash      VARCHAR(128)   NOT NULL DEFAULT '',  -- SHA-256 证明哈希
    note            VARCHAR(1024)  NOT NULL DEFAULT '',
    posted_at       DATETIME(3)             DEFAULT NULL,
    created_at      DATETIME(3)    NOT NULL,
    updated_at      DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uniq_leak_rollbacks_leak (leak_id),     -- 一案一腿
    INDEX idx_leak_rollbacks_user (user_id),
    INDEX idx_leak_rollbacks_status (status),
    INDEX idx_leak_rollbacks_convert (convert_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='泄漏回滚腿；一 LeakCase 一腿；执行后 USDT 逆记并更新 Conserve';

-- ─── convert_gates ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS convert_gates (
    id           VARCHAR(36)    NOT NULL,
    user_id      VARCHAR(64)    NOT NULL,
    asset        VARCHAR(16)    NOT NULL,
    status       VARCHAR(16)    NOT NULL,            -- open/blocked/released
    leak_id      VARCHAR(36)    NOT NULL DEFAULT '', -- 触发挡兑的 LeakCase.ID
    blocked_by   VARCHAR(64)    NOT NULL DEFAULT '',
    blocked_at   DATETIME(3)             DEFAULT NULL,
    released_by  VARCHAR(64)    NOT NULL DEFAULT '',
    released_at  DATETIME(3)             DEFAULT NULL,
    created_at   DATETIME(3)    NOT NULL,
    updated_at   DATETIME(3)    NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uniq_convert_gates_user_asset (user_id, asset),
    INDEX idx_convert_gates_status (status),
    INDEX idx_convert_gates_leak (leak_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='尘兑挡闸；一用户一资产一条记录；LeakCase 触发 blocked，收口后 released';

-- ─── dust_conv_records（只读视图，别名 dustconv.Convert 落盘表）────────────────
-- 注：如果 dustconv 有独立的 converts 表，此处只需建视图供 detector/worker 使用。
-- 若无独立表，此视图为占位，环境需替换为 dustconv 实际数据源。

CREATE OR REPLACE VIEW dust_conv_records AS
SELECT
    id,
    user_id,
    asset,
    amount,
    usdt,
    rate,
    at,
    idempotency_key
FROM dust_converts   -- dustconv 落盘表（由 dustconv 迁移创建）
WHERE 1=1;
-- 如果 dustconv 尚无持久化表，删除此视图；detector 将通过内存快照扫描。
