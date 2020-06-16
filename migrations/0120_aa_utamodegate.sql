-- UTA 全仓/经典模式切换与在途负债抵押竞态封堵引擎——数据库表定义。
--
-- 新增四张表：
--   uta_mode_generations   — 用户模式世代 CAS 计数器
--   uta_mode_leases        — ModeLease 模式切换租约（FSM 状态机记录）
--   uta_liability_binds    — LiabilityBind 在途负债/抵押世代绑定记录
--   uta_mode_drift_cases   — ModeDriftCase 模式漂移案
--   uta_mode_drift_proofs  — ModeDriftProof 模式漂移证明
--
-- 设计原则：
--   uta_mode_generations.generation 唯一递增；CAS 协议（SQL 层原子 UPDATE）：
--     UPDATE uta_mode_generations
--     SET generation = generation + 1, current_mode = ?, updated_at = NOW(3)
--     WHERE user_id = ? AND generation = ?
--     — 影响行数 = 0 → ErrStaleGen；= 1 → CAS 成功。
--
--   uta_mode_leases (user_id, trigger_ref) 唯一索引防同触发源重复租约。
--   uta_liability_binds (user_id, ref_id) 复合索引加速幂等查找。
--   uta_mode_drift_cases (user_id, asset, kind) 复合索引加速幂等查找与漂移案列表。
--   uta_mode_drift_proofs.drift_case_id 唯一索引防同案重复证明。
--
--   所有金额列 DECIMAL(36,18)；时间列 DATETIME(3)（精度 1ms）。
--   表前缀 uta_ 与既有 vip_/fee_/aa_/lo_/fx_ 体系隔离，不修改任何既有表。
--
-- 与 r11 F isobook 的隔离：
--   本文件不修改 isobook 任何表（iso_debt_ratio_leases、iso_transfer_in_binds 等）；
--   不修改 marginhook 任何表；不修改 ledger 任何核心科目表。
--
-- 主机占位（保持产品契约不变，不修改）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
--
-- 作者：KianFord
-- 日期：2020.12.21

-- ────────────────────────────────────────────────────────────────────────────
-- uta_mode_generations: 用户模式世代 CAS 计数器
-- ────────────────────────────────────────────────────────────────────────────
-- CAS 协议（SQL 层原子 UPDATE）：
--   Acquire（模式切换）:
--     UPDATE uta_mode_generations
--     SET generation = generation + 1,
--         current_mode = ?,
--         updated_at   = NOW(3)
--     WHERE user_id = ? AND generation = ?
--     -- 影响行数 = 0 → ErrStaleGen；= 1 → 成功，新世代 = old+1
--
--   Insert-first（首次）:
--     INSERT INTO uta_mode_generations (id, user_id, generation, current_mode, updated_at)
--     VALUES (?, ?, 1, ?, NOW(3))
--     ON DUPLICATE KEY UPDATE id=id
--     -- 若冲突则对方已 INSERT；重试 SELECT + UPDATE
CREATE TABLE IF NOT EXISTS uta_mode_generations (
    id           VARCHAR(36)  NOT NULL,
    user_id      VARCHAR(64)  NOT NULL,
    generation   BIGINT       NOT NULL DEFAULT 0,
    current_mode VARCHAR(16)  NOT NULL DEFAULT 'UNIFIED',   -- UNIFIED | CLASSIC
    updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_uta_mode_gen_user (user_id),
    KEY idx_uta_mode_gen_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='UTA 模式世代 CAS 计数器';

-- ────────────────────────────────────────────────────────────────────────────
-- uta_mode_leases: ModeLease 模式切换租约
-- ────────────────────────────────────────────────────────────────────────────
-- status 状态机：open → bound → switched（正常切换）
--                open → blocked（在途负债互斥，进入待切换队列）
--                blocked → open → bound → switched（worker 推进）
--
-- trigger_ref 幂等键：同 user_id + trigger_ref 不允许有多条非 switched 记录。
-- generation 为本次切换后的新世代号（CAS 递增后）。
CREATE TABLE IF NOT EXISTS uta_mode_leases (
    id           VARCHAR(36)  NOT NULL,
    user_id      VARCHAR(64)  NOT NULL,
    from_mode    VARCHAR(16)  NOT NULL,  -- UNIFIED | CLASSIC
    to_mode      VARCHAR(16)  NOT NULL,  -- UNIFIED | CLASSIC
    generation   BIGINT       NOT NULL DEFAULT 0,
    trigger_kind VARCHAR(64)  NOT NULL DEFAULT '',
    trigger_ref  VARCHAR(255) NOT NULL DEFAULT '',
    status       VARCHAR(16)  NOT NULL DEFAULT 'open',   -- open|bound|switched|blocked
    note         TEXT,
    created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_uta_lease_triggerref (user_id, trigger_ref),
    KEY idx_uta_lease_user_status (user_id, status),
    KEY idx_uta_lease_status_created (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='UTA 模式切换租约（FSM）';

-- ────────────────────────────────────────────────────────────────────────────
-- uta_liability_binds: LiabilityBind 在途负债/抵押世代绑定
-- ────────────────────────────────────────────────────────────────────────────
-- bind_kind: borrow | repay | collateral | release
-- margin_mode: 操作时生效的模式键（UNIFIED/CLASSIC/CROSS/ISOLATED）
-- generation: 操作时生效的世代号（用于切换后的过期检测）
-- status: open → released（还款/释放完成）| open → expired（TTL 过期）
-- ref_id: 关联的借还/抵押订单 ID（幂等键）
--
-- 模式切换前扫描 WHERE user_id=? AND status='open' COUNT(*)；
-- 若 > 0 则 ModeLease 进入 blocked 队列。
CREATE TABLE IF NOT EXISTS uta_liability_binds (
    id          VARCHAR(36)   NOT NULL,
    user_id     VARCHAR(64)   NOT NULL,
    bind_kind   VARCHAR(16)   NOT NULL,              -- borrow|repay|collateral|release
    currency    VARCHAR(16)   NOT NULL,
    margin_mode VARCHAR(16)   NOT NULL,              -- UNIFIED|CLASSIC|CROSS|ISOLATED
    amount      DECIMAL(36,18) NOT NULL DEFAULT 0,
    generation  BIGINT        NOT NULL DEFAULT 0,
    ref_id      VARCHAR(255)  NOT NULL DEFAULT '',   -- 借还/抵押订单 ID
    status      VARCHAR(16)   NOT NULL DEFAULT 'open', -- open|released|expired
    expires_at  DATETIME(3)   NOT NULL,
    created_at  DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_uta_bind_user_status (user_id, status),
    KEY idx_uta_bind_ref (user_id, ref_id),
    KEY idx_uta_bind_expires (status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='UTA 在途负债/抵押世代绑定';

-- ────────────────────────────────────────────────────────────────────────────
-- uta_mode_drift_cases: ModeDriftCase 模式漂移案
-- ────────────────────────────────────────────────────────────────────────────
-- kind: liability_key | collateral_key | reverse_aggregate | stale_gen_write
-- status: open → proven（附证明）→ closed（四眼收口）
--
-- 存在 open/proven 记录时：
--   同用户后续 SetMode 被拒绝（ErrDriftBlocked）
--   大额/跨模式键借出被拒绝（ErrDriftBlocked）
--
-- (user_id, asset, kind) 复合索引加速幂等查找：
--   FindOpenDriftCase WHERE user_id=? AND asset=? AND kind=? AND status='open'
CREATE TABLE IF NOT EXISTS uta_mode_drift_cases (
    id               VARCHAR(36)    NOT NULL,
    user_id          VARCHAR(64)    NOT NULL,
    asset            VARCHAR(16)    NOT NULL,   -- 漂移涉及的币种
    kind             VARCHAR(32)    NOT NULL,   -- liability_key|collateral_key|...
    old_mode         VARCHAR(16)    NOT NULL,   -- 切换前模式
    new_mode         VARCHAR(16)    NOT NULL,   -- 切换后模式
    old_generation   BIGINT         NOT NULL DEFAULT 0,
    new_generation   BIGINT         NOT NULL DEFAULT 0,
    liability_amount DECIMAL(36,18) NOT NULL DEFAULT 0,
    bind_id          VARCHAR(36)    NOT NULL DEFAULT '',
    lease_id         VARCHAR(36)    NOT NULL DEFAULT '',
    proof_id         VARCHAR(36)    NOT NULL DEFAULT '',
    status           VARCHAR(16)    NOT NULL DEFAULT 'open',  -- open|proven|closed
    confirm1_by      VARCHAR(64)    NOT NULL DEFAULT '',
    confirm2_by      VARCHAR(64)    NOT NULL DEFAULT '',
    confirm1_at      DATETIME(3),
    confirm2_at      DATETIME(3),
    note             TEXT,
    detected_at      DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at       DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_uta_drift_user_status (user_id, status),
    KEY idx_uta_drift_asset_kind (user_id, asset, kind),
    KEY idx_uta_drift_detected (status, detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='UTA 模式漂移案';

-- ────────────────────────────────────────────────────────────────────────────
-- uta_mode_drift_proofs: ModeDriftProof 模式漂移证明
-- ────────────────────────────────────────────────────────────────────────────
-- delta_digest: sha256(old_key_snapshot+"|"+new_agg_snapshot+"|"+drift_case_id)
-- 验证时重计算并；不一致 → ErrProofDigestMismatch。
-- drift_case_id 唯一索引防同案重复证明。
CREATE TABLE IF NOT EXISTS uta_mode_drift_proofs (
    id               VARCHAR(36) NOT NULL,
    drift_case_id    VARCHAR(36) NOT NULL,
    old_key_snapshot MEDIUMTEXT  NOT NULL,   -- 旧模式键下负债快照（JSON）
    new_agg_snapshot MEDIUMTEXT  NOT NULL,   -- 新模式聚合读面（JSON）
    delta_digest     VARCHAR(64) NOT NULL,   -- sha256 hex
    built_by         VARCHAR(64) NOT NULL DEFAULT '',
    verified_at      DATETIME(3) NOT NULL,
    created_at       DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_uta_proof_case (drift_case_id),
    KEY idx_uta_proof_built (built_by, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='UTA 模式漂移证明';
