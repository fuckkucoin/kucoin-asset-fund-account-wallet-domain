-- VIP 档位世代闸与漂移封堵引擎——数据库表定义。
--
-- 新增六张表：
--   vip_tier_generations  — 用户档位世代 CAS 计数器
--   vip_tier_leases       — TierLease 档位切换租约（FSM 状态机记录）
--   vip_quote_binds       — QuoteBind 报价世代绑定记录
--   vip_inflight_freezes  — 在途冻额只读观察快照
--   vip_drift_cases       — DriftCase 漂移案
--   vip_drift_proofs      — DriftProof 漂移证明
--
-- 设计原则：
--   vip_tier_generations.generation 唯一递增；CAS 协议（SQL 层原子 UPDATE）：
--     UPDATE vip_tier_generations
--     SET generation = generation + 1, last_tier = ?, updated_at = NOW(3)
--     WHERE user_id = ? AND generation = ?
--     — 影响行数 = 0 → ErrStaleGen；= 1 → CAS 成功。
--
--   vip_tier_leases (user_id, trigger_ref) 唯一索引防同触发源重复租约。
--   vip_quote_binds (user_id, status, expires_at) 复合索引加速活跃报价扫描。
--   vip_drift_cases (user_id, asset, kind) 复合索引加速幂等查找与用户漂移案列表。
--   vip_drift_proofs.drift_case_id 唯一索引防同案重复证明。
--
--   所有金额列 DECIMAL(36,18)；时间列 DATETIME(3)（精度 1ms）。
--   表前缀 vip_ 与既有 fee_/aa_/lo_/fx_ 体系隔离，不修改任何既有表。
--
-- 内部主机占位（保持产品契约不变，不修改）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
--
-- 作者：KianFord
-- 日期：2020.12.21

-- ────────────────────────────────────────────────────────────────────────────
-- vip_tier_generations: 用户档位世代 CAS 计数器
-- ────────────────────────────────────────────────────────────────────────────
-- CAS 协议（SQL 层原子 UPDATE）：
--   Acquire（抬档/降档）:
--     UPDATE vip_tier_generations
--     SET generation = generation + 1,
--         last_tier  = ?,
--         updated_at = NOW(3)
--     WHERE user_id = ? AND generation = ?
--     -- 影响行数 = 0 → ErrStaleGen；= 1 → 成功，新世代 = old+1
--
--   Insert-first（首次）:
--     INSERT INTO vip_tier_generations (id, user_id, generation, last_tier, updated_at)
--     VALUES (?, ?, 1, ?, NOW(3))
--     ON CONFLICT (user_id) DO NOTHING
--     -- 若冲突则对方已 INSERT；重试 SELECT + UPDATE
CREATE TABLE IF NOT EXISTS vip_tier_generations (
    id         VARCHAR(64)  NOT NULL,
    user_id    VARCHAR(64)  NOT NULL,
    generation BIGINT       NOT NULL DEFAULT 0,
    last_tier  VARCHAR(16)  NOT NULL DEFAULT 'standard',
    updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_vip_tier_gen_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='VIP 档位世代 CAS 计数器，抬档/降档操作原子递增';

-- ────────────────────────────────────────────────────────────────────────────
-- vip_tier_leases: TierLease 档位切换租约
-- ────────────────────────────────────────────────────────────────────────────
-- FSM 状态转移：
--   open → bound → switched   (正常路径)
--   open → blocked            (在途冻额/Quote 互斥)
--   blocked → open → bound → switched (待切换队列推进)
--
-- trigger_kind: 'pin' | 'clear_pin' | 'volume'
-- trigger_ref:  对应 Pin/Fill 的唯一标识，用于幂等保护
CREATE TABLE IF NOT EXISTS vip_tier_leases (
    id           VARCHAR(64)  NOT NULL,
    user_id      VARCHAR(64)  NOT NULL,
    from_tier    VARCHAR(16)  NOT NULL,
    to_tier      VARCHAR(16)  NOT NULL,
    generation   BIGINT       NOT NULL DEFAULT 0,
    trigger_kind VARCHAR(16)  NOT NULL,
    trigger_ref  VARCHAR(128) NOT NULL DEFAULT '',
    status       VARCHAR(16)  NOT NULL DEFAULT 'open',
    block_reason VARCHAR(255) NOT NULL DEFAULT '',
    drift_case_id VARCHAR(64) NOT NULL DEFAULT '',
    bound_at     DATETIME(3)  NULL,
    switched_at  DATETIME(3)  NULL,
    blocked_at   DATETIME(3)  NULL,
    created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    -- 同一用户+触发源只允许一条活跃租约（幂等保护）。
    UNIQUE KEY uq_vip_lease_trigger (user_id, trigger_ref),
    KEY idx_vip_lease_user_status (user_id, status),
    KEY idx_vip_lease_status (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='VIP 档位切换租约 FSM，状态 open/bound/switched/blocked';

-- ────────────────────────────────────────────────────────────────────────────
-- vip_quote_binds: QuoteBind 报价世代绑定记录
-- ────────────────────────────────────────────────────────────────────────────
-- 颁发时快照 generation；结算时当前世代；不等则 ErrQuoteGenMismatch。
-- status: 'open' | 'settled' | 'voided' | 'expired'
CREATE TABLE IF NOT EXISTS vip_quote_binds (
    id          VARCHAR(64)    NOT NULL,
    user_id     VARCHAR(64)    NOT NULL,
    asset       VARCHAR(16)    NOT NULL,
    gross       DECIMAL(36,18) NOT NULL DEFAULT 0,
    fee         DECIMAL(36,18) NOT NULL DEFAULT 0,
    net         DECIMAL(36,18) NOT NULL DEFAULT 0,
    bps         INT            NOT NULL DEFAULT 0,
    tier        VARCHAR(16)    NOT NULL DEFAULT 'standard',
    generation  BIGINT         NOT NULL DEFAULT 0,
    lease_id    VARCHAR(64)    NOT NULL DEFAULT '',
    status      VARCHAR(16)    NOT NULL DEFAULT 'open',
    expires_at  DATETIME(3)    NOT NULL,
    settled_at  DATETIME(3)    NULL,
    voided_at   DATETIME(3)    NULL,
    created_at  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    -- 活跃报价扫描（worker TTL 巡检 + 降档互斥检查）。
    KEY idx_vip_qb_user_status (user_id, status, expires_at),
    KEY idx_vip_qb_status_expires (status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='QuoteBind 报价世代绑定；generation 与 TierGeneration 验证';

-- ────────────────────────────────────────────────────────────────────────────
-- vip_inflight_freezes: 在途冻额只读观察快照
-- ────────────────────────────────────────────────────────────────────────────
-- 由出金模块 push；viptiergate 只读消费。released_at NULL 表示仍在途。
CREATE TABLE IF NOT EXISTS vip_inflight_freezes (
    id          VARCHAR(64)    NOT NULL,
    user_id     VARCHAR(64)    NOT NULL,
    asset       VARCHAR(16)    NOT NULL,
    amount      DECIMAL(36,18) NOT NULL DEFAULT 0,
    bps         INT            NOT NULL DEFAULT 0,
    tier        VARCHAR(16)    NOT NULL DEFAULT 'standard',
    generation  BIGINT         NOT NULL DEFAULT 0,
    created_at  DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    released_at DATETIME(3)    NULL,
    PRIMARY KEY (id),
    -- 活跃冻额查询（降档互斥检查的核心索引）。
    KEY idx_vip_freeze_user_active (user_id, released_at),
    KEY idx_vip_freeze_gen (user_id, generation)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='在途出金冻额只读快照；released_at IS NULL 为活跃状态';

-- ────────────────────────────────────────────────────────────────────────────
-- vip_drift_cases: DriftCase 漂移案
-- ────────────────────────────────────────────────────────────────────────────
-- FSM 状态转移：
--   open → proven → closed
--
-- kind: 'bps_stale' | 'freeze_stale' | 'bps_reverse' | 'quote_gen_stale'
-- 存在 open/proven 案件时，同用户后续抬档结算被挡写（ErrDriftBlocked）。
CREATE TABLE IF NOT EXISTS vip_drift_cases (
    id               VARCHAR(64)    NOT NULL,
    user_id          VARCHAR(64)    NOT NULL,
    asset            VARCHAR(16)    NOT NULL,
    kind             VARCHAR(32)    NOT NULL,
    observed_tier    VARCHAR(16)    NOT NULL DEFAULT 'standard',
    expected_tier    VARCHAR(16)    NOT NULL DEFAULT 'standard',
    observed_bps     INT            NOT NULL DEFAULT 0,
    expected_bps     INT            NOT NULL DEFAULT 0,
    amount           DECIMAL(36,18) NOT NULL DEFAULT 0,
    quote_bind_id    VARCHAR(64)    NOT NULL DEFAULT '',
    lease_id         VARCHAR(64)    NOT NULL DEFAULT '',
    generation       BIGINT         NOT NULL DEFAULT 0,
    status           VARCHAR(16)    NOT NULL DEFAULT 'open',
    proof_id         VARCHAR(64)    NOT NULL DEFAULT '',
    first_confirm_by VARCHAR(64)    NOT NULL DEFAULT '',
    second_confirm_by VARCHAR(64)   NOT NULL DEFAULT '',
    first_confirm_at  DATETIME(3)   NULL,
    second_confirm_at DATETIME(3)   NULL,
    closed_at        DATETIME(3)    NULL,
    note             TEXT           NULL,
    detected_at      DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at       DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    -- 幂等查找：同用户+资产+种类的 open 案件唯一（防并发重复开案）。
    KEY idx_vip_dc_open_lookup (user_id, asset, kind, status),
    -- 用户漂移案列表查询。
    KEY idx_vip_dc_user_status (user_id, status),
    -- 全局 open 案件巡检（worker）。
    KEY idx_vip_dc_status (status, detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='DriftCase 漂移案 FSM：open→proven→closed；阻塞同用户后续结算';

-- ────────────────────────────────────────────────────────────────────────────
-- vip_drift_proofs: DriftProof 漂移证明
-- ────────────────────────────────────────────────────────────────────────────
-- SHA-256 摘要字段 digest 用于防篡改验签。
-- status: 'pending' | 'verified' | 'failed'
-- 每个 DriftCase 最多一条 verified 证明（drift_case_id 唯一约束）。
CREATE TABLE IF NOT EXISTS vip_drift_proofs (
    id           VARCHAR(64)    NOT NULL,
    drift_case_id VARCHAR(64)   NOT NULL,
    user_id      VARCHAR(64)    NOT NULL,
    asset        VARCHAR(16)    NOT NULL,
    observed_bps INT            NOT NULL DEFAULT 0,
    expected_bps INT            NOT NULL DEFAULT 0,
    amount       DECIMAL(36,18) NOT NULL DEFAULT 0,
    quote_bind_id VARCHAR(64)   NOT NULL DEFAULT '',
    lease_id     VARCHAR(64)    NOT NULL DEFAULT '',
    generation   BIGINT         NOT NULL DEFAULT 0,
    digest       VARCHAR(64)    NOT NULL DEFAULT '',
    status       VARCHAR(16)    NOT NULL DEFAULT 'pending',
    verified_at  DATETIME(3)    NULL,
    created_at   DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    -- 每个案件只允许一条 verified 证明（幂等 + 防重复构建）。
    UNIQUE KEY uq_vip_proof_case (drift_case_id),
    KEY idx_vip_proof_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='DriftProof 漂移证明；digest 为 SHA-256 摘要，防篡改验签';
