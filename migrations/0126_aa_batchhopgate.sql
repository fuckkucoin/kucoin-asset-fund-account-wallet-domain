-- 组合账户批量 hop 部分失败残留与续跳互斥收口引擎——数据库表定义。
--
-- 新增四张表：
--   portfolio_batch_generations  — BatchGeneration 用户资产 CAS 世代计数器
--   portfolio_batch_residuals    — BatchResidualCase 批量 hop 部分失败残留案
--   portfolio_resume_leases      — ResumeLease 续跳互斥世代租约
--   portfolio_hop_proofs         — HopOccupyProof 部分守恒占用证明
--
-- 设计原则：
--   portfolio_batch_generations.(user_id, asset) 唯一索引防重复初始化；
--   CAS 协议（SQL 层原子 UPDATE）：
--     UPDATE portfolio_batch_generations
--     SET generation = generation + 1, status = 'active', updated_at = NOW(3)
--     WHERE user_id = ? AND asset = ? AND generation = ? AND status = 'idle'
--     — 影响行数 = 0 → ErrStaleGeneration 或 ErrConcurrentBatch；= 1 → 成功。
--
--   portfolio_batch_residuals.(user_id, asset, status) 复合索引加速并行互斥查找：
--     SELECT ... WHERE user_id=? AND asset=? AND status IN ('open','proven','resuming')
--
--   portfolio_resume_leases.(residual_case_id, status) 复合索引加速活跃租约查找：
--     SELECT ... WHERE residual_case_id=? AND status IN ('open','bound')
--
--   portfolio_hop_proofs.residual_case_id 唯一索引防同案重复证明。
--
--   所有金额列 DECIMAL(36,18)；时间列 DATETIME(3)（精度 1ms）。
--   表前缀 portfolio_ 与既有 uta_/vip_/fee_/aa_/lo_/fx_ 体系隔离，不修改任何既有表。
--
-- 与 r12 G acctpol / r6 D xferloop / r10 D wdbatch 的隔离：
--   本文件不修改 acctpol、xferloop、wdbatch 任何表；
--   不修改 ledger 任何核心科目表（aa_accounts、aa_account_books 等）。
--
-- 主机占位（保持产品契约不变，不修改）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
--
-- 作者：KianFord
-- 日期：2020.12.13

-- ─────────────────────────────────────────────────────────────────────────────
-- portfolio_batch_generations: BatchGeneration 用户资产 CAS 世代计数器
-- ─────────────────────────────────────────────────────────────────────────────
--
-- CAS 协议（SQL 层原子 UPDATE）：
--
--   Acquire（开启新 BatchHop）:
--     UPDATE portfolio_batch_generations
--     SET generation = generation + 1, status = 'active', updated_at = NOW(3)
--     WHERE user_id = ? AND asset = ? AND generation = ? AND status = 'idle'
--     -- 影响行数 = 0 → ErrStaleGeneration（world state 已变）或 ErrConcurrentBatch（已有活跃批次）
--     -- 影响行数 = 1 → 成功，新世代 = old+1
--
--   Release（BatchHop 完成或失败收口后）:
--     UPDATE portfolio_batch_generations
--     SET status = 'idle', updated_at = NOW(3)
--     WHERE user_id = ? AND asset = ? AND generation = ?
--
--   Insert-first（首次）:
--     INSERT INTO portfolio_batch_generations (id, user_id, asset, generation, status, updated_at)
--     VALUES (?, ?, ?, 0, 'idle', NOW(3))
--     ON DUPLICATE KEY UPDATE id=id
CREATE TABLE IF NOT EXISTS portfolio_batch_generations (
    id          VARCHAR(36)  NOT NULL,
    user_id     VARCHAR(64)  NOT NULL,
    asset       VARCHAR(16)  NOT NULL,
    generation  BIGINT       NOT NULL DEFAULT 0,
    status      VARCHAR(8)   NOT NULL DEFAULT 'idle',  -- idle | active
    updated_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_pbg_user_asset (user_id, asset),
    KEY idx_pbg_status (status),
    KEY idx_pbg_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='BatchHop 用户资产 CAS 世代计数器';

-- ─────────────────────────────────────────────────────────────────────────────
-- portfolio_batch_residuals: BatchResidualCase 批量 hop 部分失败残留案
-- ─────────────────────────────────────────────────────────────────────────────
--
-- status 状态机：
--   open      → proven   : 占用证明已构建并附于残留案（Prove）
--   open      → blocked  : 审核拒绝（Block）
--   proven    → resuming : 绑定续跳互斥租约（BindLease）
--   proven    → blocked  : 审核拒绝（Block）
--   resuming  → closed   : 续跳完成（Close / ConfirmClose × 2）
--   resuming  → blocked  : 审核拒绝（Block）
--
-- 非终态（open/proven/resuming）时：
--   禁止同 user_id + asset 发起新 BatchHop（ErrConcurrentBatch）。
--
-- succeeded_hops: JSON 格式存储已成功 hop 摘要列表（[]HopSummary）。
-- failed_reason: 原始错误字符串（不解析）。
-- before_root / partial_root: sha256 摘要（见 SnapshotRoot）。
CREATE TABLE IF NOT EXISTS portfolio_batch_residuals (
    id              VARCHAR(36)    NOT NULL,
    user_id         VARCHAR(64)    NOT NULL,
    asset           VARCHAR(16)    NOT NULL,
    generation      BIGINT         NOT NULL DEFAULT 0,
    total_items     INT            NOT NULL DEFAULT 0,
    fail_index      INT            NOT NULL DEFAULT 0,
    succeeded_hops  MEDIUMTEXT     NOT NULL DEFAULT '',  -- JSON []HopSummary
    failed_reason   TEXT           NOT NULL DEFAULT '',
    before_root     VARCHAR(64)    NOT NULL DEFAULT '',  -- sha256 hex
    partial_root    VARCHAR(64)    NOT NULL DEFAULT '',  -- sha256 hex
    status          VARCHAR(12)    NOT NULL DEFAULT 'open',   -- open|proven|resuming|closed|blocked
    proof_id        VARCHAR(36)    NOT NULL DEFAULT '',
    lease_id        VARCHAR(36)    NOT NULL DEFAULT '',
    confirm1_by     VARCHAR(64)    NOT NULL DEFAULT '',
    confirm2_by     VARCHAR(64)    NOT NULL DEFAULT '',
    confirm1_at     DATETIME(3)    NULL,
    confirm2_at     DATETIME(3)    NULL,
    note            TEXT,
    detected_at     DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pbr_user_asset_status (user_id, asset, status),
    KEY idx_pbr_user_status (user_id, status),
    KEY idx_pbr_status_detected (status, detected_at),
    KEY idx_pbr_generation (user_id, asset, generation)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='BatchHop 部分失败残留案（FSM）';

-- ─────────────────────────────────────────────────────────────────────────────
-- portfolio_resume_leases: ResumeLease 续跳互斥世代租约
-- ─────────────────────────────────────────────────────────────────────────────
--
-- status 状态机：
--   open    → bound   : 绑定续跳 idem 键（Bind）
--   open    → expired : worker 标记过期（ExpireStale）
--   bound   → done    : 续跳完成（Done）
--
-- 同一 residual_case_id 同一时刻至多一条 open/bound 租约（FindActiveLease 互斥）。
-- generation 须与 BatchResidualCase.generation 匹配，失配视为过期令牌。
-- resume_from 指定从第几项开始续跳（须 >= BatchResidualCase.fail_index）。
-- expires_at 由颁发时 NOW() + ResumeLeaseTTL（30min）计算；
--   worker LeaseReaper 周期扫描 status='open' AND expires_at < NOW() 标记 expired。
CREATE TABLE IF NOT EXISTS portfolio_resume_leases (
    id               VARCHAR(36)  NOT NULL,
    residual_case_id VARCHAR(36)  NOT NULL,
    user_id          VARCHAR(64)  NOT NULL,
    asset            VARCHAR(16)  NOT NULL,
    generation       BIGINT       NOT NULL DEFAULT 0,
    resume_from      INT          NOT NULL DEFAULT 0,
    bound_idem       VARCHAR(255) NOT NULL DEFAULT '',  -- 绑定的续跳幂等键
    status           VARCHAR(10)  NOT NULL DEFAULT 'open',  -- open|bound|done|expired
    expires_at       DATETIME(3)  NOT NULL,
    confirmed_by     VARCHAR(64)  NOT NULL DEFAULT '',
    confirmed_at     DATETIME(3)  NULL,
    note             TEXT,
    created_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_prl_case_status (residual_case_id, status),
    KEY idx_prl_user_status (user_id, status),
    KEY idx_prl_expires (status, expires_at),
    KEY idx_prl_bound_idem (bound_idem)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='续跳互斥世代租约（FSM）';

-- ─────────────────────────────────────────────────────────────────────────────
-- portfolio_hop_proofs: HopOccupyProof 部分守恒占用证明
-- ─────────────────────────────────────────────────────────────────────────────
--
-- Digest 计算：sha256(before_root + "|" + partial_root + "|" + residual_case_id)
-- 验证时重计算并；不一致 → ErrProofDigestMismatch。
-- residual_case_id 唯一索引防同案重复证明。
--
-- subject_totals: JSON 格式存储各科目资金变动合计（[]BookTotal）。
-- conserve_ok: 1 表示已成功子集守恒；0 表示异常（需告警，不自动修复）。
CREATE TABLE IF NOT EXISTS portfolio_hop_proofs (
    id               VARCHAR(36)    NOT NULL,
    residual_case_id VARCHAR(36)    NOT NULL,
    user_id          VARCHAR(64)    NOT NULL,
    asset            VARCHAR(16)    NOT NULL,
    before_root      VARCHAR(64)    NOT NULL DEFAULT '',
    partial_root     VARCHAR(64)    NOT NULL DEFAULT '',
    subject_totals   MEDIUMTEXT     NOT NULL DEFAULT '',  -- JSON []BookTotal
    conserve_ok      TINYINT(1)     NOT NULL DEFAULT 0,
    digest           VARCHAR(64)    NOT NULL DEFAULT '',  -- sha256 hex
    built_by         VARCHAR(64)    NOT NULL DEFAULT '',
    verified_at      DATETIME(3)    NOT NULL,
    created_at       DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_php_case (residual_case_id),
    KEY idx_php_user (user_id),
    KEY idx_php_built (built_by, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='部分守恒占用证明';
