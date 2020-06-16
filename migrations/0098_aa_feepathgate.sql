-- 手续费收取与返佣贷记双路径互斥封堵引擎 — 数据库表定义。
--
-- 新增五张表：
--   fee_path_leases       — PathLease 路径租约（FSM 状态机记录）
--   fee_path_generations  — PathGeneration 世代 CAS 计数器
--   fee_dual_hits         — DualHit 双路径贷记命中记录
--   fee_hedge_proofs      — HedgeProof 费返对冲证明
--   fee_replay_blocks     — ReplayBlock 幂等冲突封堵记录
--
-- 设计原则：
--   fee_path_generations.generation 唯一递增；
--   InsertLease 在 SQL 层做：
--     UPDATE fee_path_generations SET generation=generation+1, locked_by=?, locked_kind=?
--     WHERE biz_key=? AND generation=?
--   若影响行数=0 则 ErrStaleGen（CAS 失败）。
--   fee_path_leases (business_key, kind) 唯一索引防同路径重复写入。
--   fee_dual_hits.business_key 唯一索引防重复发现。
--   fee_replay_blocks.idem_key 唯一索引为幂等封堵的最后防线。
--
--   所有金额列 DECIMAL(36,18)；时间列 DATETIME(3)（精度 1ms）。
--   表前缀 fee_ 与既有 aa_/lo_/fx_/sr_ 体系隔离，不修改任何既有表。
--
-- 内部主机占位（保持产品契约不变，不修改）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
--
-- 作者：Kinsley
-- 日期：2025.03.18

-- ────────────────────────────────────────────────────────────────────────────
-- fee_path_generations: 业务键世代 CAS 计数器
-- ────────────────────────────────────────────────────────────────────────────
-- CAS 协议（SQL 层原子 UPDATE）：
--   Acquire:
--     UPDATE fee_path_generations
--     SET generation=generation+1, locked_by=?, locked_kind=?, updated_at=NOW(3)
--     WHERE biz_key=? AND generation=?
--     -- 影响行数=0 → ErrStaleGen；=1 → 成功
--
--   Insert-first:
--     INSERT IGNORE INTO fee_path_generations(id, biz_key, generation, locked_by, locked_kind, updated_at)
--     VALUES(?, ?, 1, ?, ?, NOW(3))
--     -- 若已存在则走 UPDATE 路径
--
-- 此表的 generation 列是整个路径互斥机制的核心原子量；
-- 任何节点在提交 PathLease 之前必须先通过此表的 CAS 竞争。

CREATE TABLE IF NOT EXISTS fee_path_generations (
  id           VARCHAR(64)   NOT NULL PRIMARY KEY
    COMMENT '行 ID（UUID）',

  biz_key      VARCHAR(255)  NOT NULL
    COMMENT '业务键（user_id|asset|base_idem）',

  generation   BIGINT        NOT NULL DEFAULT 0
    COMMENT '当前世代号（从 0 开始，每次成功 Acquire 递增至 1,2,3…）',

  locked_by    VARCHAR(64)   NOT NULL DEFAULT ''
    COMMENT '持有锁的 lease_id',

  locked_kind  VARCHAR(16)   NOT NULL DEFAULT ''
    COMMENT '持有锁的路径 Kind：take / rebate',

  updated_at   DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    ON UPDATE CURRENT_TIMESTAMP(3)
    COMMENT '最近一次 CAS 成功时刻',

  UNIQUE KEY uk_biz_key (biz_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='费路径世代 CAS 计数器——路径互斥的核心原子量';

-- ────────────────────────────────────────────────────────────────────────────
-- fee_path_leases: PathLease 路径租约 FSM 记录
-- ────────────────────────────────────────────────────────────────────────────
-- FSM 状态流转：
--   open → bound     （Bind：绑定 credit_id / ledger_id）
--   bound → settled  （Settle：对冲证明通过）
--   open/bound → conflicted （MarkConflicted：双贷发现或落账失败）
--
-- (business_key, kind) 上的唯一索引保证同路径同 biz_key 只有一行活跃记录；
-- 若 status 为 settled/conflicted 则允许历史重复（应用层保证只有终态行
-- 才允许同 biz_key+kind 的新行插入）。
--
-- 实际上应用层 CheckAndAcquire 先经过 fee_path_generations CAS，
-- 因此 (business_key, kind) 唯一冲突只作为兜底防线。

CREATE TABLE IF NOT EXISTS fee_path_leases (
  id            VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '租约 ID（UUID，同时作为 GateReq.LeaseID）',

  business_key  VARCHAR(255)   NOT NULL
    COMMENT '路径互斥业务键',

  kind          VARCHAR(16)    NOT NULL
    COMMENT '路径类型：take / rebate',

  asset         VARCHAR(16)    NOT NULL
    COMMENT '资产标识（大写，如 USDT / BTC）',

  gross         DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT '交易 gross 金额（来自 feecredit.PostReq.Gross）',

  fee           DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT '手续费金额（feecredit.Credit.Fee）',

  net           DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT '净额（feecredit.Credit.Net）',

  generation    BIGINT         NOT NULL DEFAULT 0
    COMMENT '获取租约时的 PathGeneration.generation 值（用于审计 CAS 轮次）',

  worker_id     VARCHAR(255)   NOT NULL DEFAULT ''
    COMMENT '持有租约的调用方身份（hostname:pid:uuid 格式）',

  credit_id     VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'feecredit.Credit.ID（Bind 后填写）',

  ledger_id     VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'ledger.Transaction.ID（Rebate 路径落账后填写；Take 路径无 ledger 时为空）',

  proof_id      VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'HedgeProof.ID（Settle 后填写）',

  status        VARCHAR(16)    NOT NULL DEFAULT 'open'
    COMMENT '租约状态：open / bound / settled / conflicted',

  conflict_at   DATETIME(3)    NULL
    COMMENT '进入 conflicted 状态的时刻（双贷发现或落账失败）',

  settled_at    DATETIME(3)    NULL
    COMMENT '进入 settled 状态的时刻（对冲证明通过）',

  created_at    DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '租约创建时刻',

  updated_at    DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    ON UPDATE CURRENT_TIMESTAMP(3)
    COMMENT '最近一次状态变更时刻',

  INDEX idx_biz_status (business_key, status),
  INDEX idx_status_created (status, created_at),
  INDEX idx_credit_id (credit_id),
  INDEX idx_ledger_id (ledger_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='费路径租约 FSM——PathLease 状态机记录';

-- ────────────────────────────────────────────────────────────────────────────
-- fee_dual_hits: DualHit 双路径贷记命中记录
-- ────────────────────────────────────────────────────────────────────────────
-- 发现条件：同 business_key 的 Take + Rebate 两条 PathLease
--   同时处于 bound/settled 状态且两侧均有 ledger_id。
--
-- severity 分级：
--   high   — 双贷金额均非零且无任何证明，或 fee_take ≠ fee_rebate 失衡
--   medium — 一侧已有证明或金额差异极小
--   low    — 两侧 fee < minFeeThreshold（极小金额）
--
-- 生命周期：
--   open   → proven  （对冲证明固化后）
--   proven → closed  （人工双人确认后）
-- open 状态的 DualHit 不允许继续落账（路径已为 conflicted）。

CREATE TABLE IF NOT EXISTS fee_dual_hits (
  id                VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '命中记录 ID（UUID）',

  business_key      VARCHAR(255)   NOT NULL
    COMMENT '触发双贷的业务键',

  asset             VARCHAR(16)    NOT NULL
    COMMENT '资产标识',

  take_lease_id     VARCHAR(64)    NOT NULL
    COMMENT 'Take 路径的 PathLease.ID',

  take_credit_id    VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'Take 路径的 Credit.ID',

  take_ledger_id    VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'Take 路径的 LedgerID（可为空：Take 路径无 ledger 贷记）',

  take_gross        DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Take 路径的 gross 金额',

  take_fee          DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Take 路径的 fee 金额',

  take_net          DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Take 路径的 net 金额',

  rebate_lease_id   VARCHAR(64)    NOT NULL
    COMMENT 'Rebate 路径的 PathLease.ID',

  rebate_credit_id  VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'Rebate 路径的 Credit.ID',

  rebate_ledger_id  VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'Rebate 路径的 LedgerID（Rebate 侧必须有）',

  rebate_gross      DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Rebate 路径的 gross 金额',

  rebate_fee        DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Rebate 路径的 fee 金额',

  rebate_net        DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Rebate 路径的 net 金额',

  severity          VARCHAR(8)     NOT NULL DEFAULT 'high'
    COMMENT '严重程度：high / medium / low',

  status            VARCHAR(8)     NOT NULL DEFAULT 'open'
    COMMENT '处理状态：open / proven / closed',

  proof_id          VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '关联的 HedgeProof.ID（proven 后填写）',

  detected_at       DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '发现时刻',

  resolved_at       DATETIME(3)    NULL
    COMMENT '人工确认关闭时刻',

  resolved_by       VARCHAR(255)   NOT NULL DEFAULT ''
    COMMENT '执行关闭操作的 ops 账号',

  UNIQUE KEY uk_biz_key (business_key),
  INDEX idx_status_detected (status, detected_at),
  INDEX idx_severity_status (severity, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='双路径贷记命中记录——需人工双人确认后关闭';

-- ────────────────────────────────────────────────────────────────────────────
-- fee_hedge_proofs: HedgeProof 费返对冲证明
-- ────────────────────────────────────────────────────────────────────────────
-- 证明字段 digest = SHA256(business_key ‖ asset ‖ 六个金额字段 ‖ sorted(ledger_ids))
-- pending  → verified （Verify 成功；两条租约同时推进至 settled）
-- pending  → failed   （Verify 失败；digest 不符或对冲失衡）
--
-- 对冲平衡条件（仅在 Build 时校验，Verify 时再次计算 digest）：
--   fee_take == fee_rebate（同资产同金额，否则 ErrHedgeImbalance）
--   LedgerIDs 集合非空
--
-- 注意：gross_take ≠ gross_rebate 是允许的（gross 含净额，两侧含义不同）；
-- 互斥的核心度量是 fee（手续费）金额的对称性。

CREATE TABLE IF NOT EXISTS fee_hedge_proofs (
  id               VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '证明 ID（UUID）',

  business_key     VARCHAR(255)   NOT NULL
    COMMENT '对应的业务键',

  asset            VARCHAR(16)    NOT NULL
    COMMENT '资产标识',

  gross_take       DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Take 路径 gross 金额',

  fee_take         DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Take 路径 fee 金额',

  net_take         DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Take 路径 net 金额',

  gross_rebate     DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Rebate 路径 gross 金额',

  fee_rebate       DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Rebate 路径 fee 金额',

  net_rebate       DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT 'Rebate 路径 net 金额',

  ledger_ids       TEXT           NOT NULL DEFAULT ''
    COMMENT 'LedgerID 集合（逗号分隔，排序后存储；用于 digest 计算）',

  digest           VARCHAR(64)    NOT NULL
    COMMENT 'SHA-256 摘要（HedgeProof.ComputeDigest() 结果）',

  status           VARCHAR(12)    NOT NULL DEFAULT 'pending'
    COMMENT '验证状态：pending / verified / failed',

  take_lease_id    VARCHAR(64)    NOT NULL
    COMMENT '关联 Take PathLease.ID',

  rebate_lease_id  VARCHAR(64)    NOT NULL
    COMMENT '关联 Rebate PathLease.ID',

  verified_at      DATETIME(3)    NULL
    COMMENT 'Verify 通过时刻',

  created_at       DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '证明创建时刻',

  INDEX idx_biz_key (business_key),
  INDEX idx_take_rebate (take_lease_id, rebate_lease_id),
  INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='费返对冲证明——存档 SHA-256 摘要与对冲金额';

-- ────────────────────────────────────────────────────────────────────────────
-- fee_replay_blocks: ReplayBlock 幂等冲突封堵记录
-- ────────────────────────────────────────────────────────────────────────────
-- 当同一 idem_key 以不同 (Kind, Asset, Gross) 重用时，
-- 将冲突请求记入此表，并对调用方返回 ErrReplayConflict。
-- idem_key 唯一索引是最后防线；应用层 ReplayFence 先于 SQL 层做内存校验。
--
-- 此表不阻断任何已提交的 ledger 事务；
-- 仅作为幂等冲突的审计证据与后续人工溯源依据。

CREATE TABLE IF NOT EXISTS fee_replay_blocks (
  id            VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '行 ID（UUID）',

  idem_key      VARCHAR(128)   NOT NULL
    COMMENT '被封堵的幂等键',

  business_key  VARCHAR(255)   NOT NULL DEFAULT ''
    COMMENT '关联的业务键',

  orig_kind     VARCHAR(16)    NOT NULL DEFAULT ''
    COMMENT '首次注册时的 Kind',

  orig_asset    VARCHAR(16)    NOT NULL DEFAULT ''
    COMMENT '首次注册时的 Asset',

  orig_gross    DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT '首次注册时的 Gross',

  req_kind      VARCHAR(16)    NOT NULL DEFAULT ''
    COMMENT '本次冲突请求的 Kind',

  req_asset     VARCHAR(16)    NOT NULL DEFAULT ''
    COMMENT '本次冲突请求的 Asset',

  req_gross     DECIMAL(36,18) NOT NULL DEFAULT 0
    COMMENT '本次冲突请求的 Gross',

  blocked_at    DATETIME(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '封堵时刻',

  UNIQUE KEY uk_idem_key (idem_key),
  INDEX idx_blocked_at (blocked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='幂等复用冲突封堵记录——跨 Kind 三元组不等即封堵';
