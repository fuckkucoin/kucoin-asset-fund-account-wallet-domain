-- 资金编排补偿失败残留发现与强制收口引擎 — 数据库表定义。
--
-- 新增八张表：
--   sr_residue_runs     — ResidueRun 生命周期状态机
--   sr_residue_items    — 残留科目明细（每条对应一个 ledger hop）
--   sr_residue_reports  — 扫描报告（每次 Detect 产出一份）
--   sr_closeout_plans   — 强制收口计划（一个 ResidueRun 至多一份活跃 Plan）
--   sr_closeout_legs    — 收口腿明细（冲正 / 挂账 / 挡写封印）
--   sr_close_proofs     — 收口终态证明（意图集合 ⊕ 残留集合 ⊕ 腿哈希）
--   sr_mutex_gens       — 补偿/收口互斥世代锁
--   sr_idem_blocks      — 幂等拒写记录（FenceBlock 腿写入，阻止 saga 续写）
--
-- 设计原则：
--   sr_mutex_gens.saga_run_id 唯一约束实现单一 saga Run 互斥；
--   sr_idem_blocks.idem_key 唯一约束为封堵后的最后防线；
--   所有金额列 DECIMAL(36,18)；时间列 DATETIME(3)（毫秒精度）。
--   表前缀 sr_ 与既有 aa_/lo_/fx_ 体系隔离，不修改任何既有表。
--
-- 内部主机占位（保持产品契约不变，不修改）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 从库）
--
-- 作者：Kinsley
-- 日期：2020.12.18

-- ─────────────────────────────────────────────────────────────────────────────
-- sr_residue_runs: ResidueRun 生命周期
-- ─────────────────────────────────────────────────────────────────────────────
-- Run FSM：
--   partial_failed → residue_open → closing → closed
--                                           → escalated
--
-- MutexGen 持锁协议见 sr_mutex_gens 说明。
-- close_proof_id 非空且对应 sr_close_proofs.is_valid=1 才可标 closed。

CREATE TABLE IF NOT EXISTS sr_residue_runs (
  id                VARCHAR(64)     NOT NULL PRIMARY KEY
    COMMENT 'UUID（应用层生成）',

  saga_run_id       VARCHAR(64)     NOT NULL
    COMMENT '关联的 safesaga.Run.ID（只读消费，不持写权限）',

  user_id           VARCHAR(128)    NOT NULL DEFAULT ''
    COMMENT '资金归属用户 ID',

  asset             VARCHAR(32)     NOT NULL DEFAULT ''
    COMMENT '残留资产标识（如 BTC/ETH/USDT）',

  partial_done_steps TEXT            NOT NULL DEFAULT '[]'
    COMMENT '补偿中断时的 Partial.Done 步骤快照（JSON array of string）',

  partial_cause     VARCHAR(1024)   NOT NULL DEFAULT ''
    COMMENT '补偿中断原因（来自 Partial.Cause）',

  state             VARCHAR(32)     NOT NULL DEFAULT 'partial_failed'
    COMMENT 'Run 状态机：partial_failed|residue_open|closing|closed|escalated',

  severity          VARCHAR(16)     NOT NULL DEFAULT 'low'
    COMMENT '残留严重级别：low|medium|high|critical',

  mutex_generation  BIGINT          NOT NULL DEFAULT 0
    COMMENT '当前持锁世代号（从 sr_mutex_gens 同步，用于乐观并发校验）',

  closeout_plan_id  VARCHAR(64)     NULL
    COMMENT '关联的活跃 CloseoutPlan.ID',

  close_proof_id    VARCHAR(64)     NULL
    COMMENT '收口终态证明 CloseProof.ID；非空才可标 closed',

  escalated_at      DATETIME(3)     NULL
    COMMENT '升级为 escalated 的时间',

  closed_at         DATETIME(3)     NULL
    COMMENT '标记 closed 的时间',

  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '创建时间',

  updated_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '最后更新时间',

  UNIQUE KEY uk_sr_residue_runs_saga_run_id (saga_run_id),
  KEY idx_sr_residue_runs_state (state),
  KEY idx_sr_residue_runs_severity (severity),
  KEY idx_sr_residue_runs_user_id (user_id),
  KEY idx_sr_residue_runs_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='ResidueRun 生命周期状态机（资金编排补偿失败残留）';

-- ─────────────────────────────────────────────────────────────────────────────
-- sr_residue_items: 残留科目明细
-- ─────────────────────────────────────────────────────────────────────────────
-- 每条 ResidueItem 对应 safesaga.Partial.Done 中一个步骤的 ledger hop 落账。
-- is_confirmed=1 由双人授权确认；未确认的 item 不纳入 CloseoutPlan。

CREATE TABLE IF NOT EXISTS sr_residue_items (
  id                VARCHAR(64)     NOT NULL PRIMARY KEY
    COMMENT 'UUID',

  residue_run_id    VARCHAR(64)     NOT NULL
    COMMENT '所属 ResidueRun.ID',

  step              VARCHAR(32)     NOT NULL
    COMMENT '对应的 safesaga.Step 名称（如 freeze / move / sign）',

  idem_key          VARCHAR(256)    NOT NULL
    COMMENT '该步骤的 ledger 幂等键（如 {run_idem}-move）',

  ledger_entry_id   VARCHAR(64)     NULL
    COMMENT '对应的 ledger.Transfer.ID（已落账时填入）',

  amount            DECIMAL(36,18)  NOT NULL DEFAULT 0
    COMMENT '残留金额',

  asset             VARCHAR(32)     NOT NULL DEFAULT ''
    COMMENT '资产标识',

  direction         VARCHAR(64)     NOT NULL DEFAULT ''
    COMMENT 'ledger.Direction 值快照',

  hop_state         VARCHAR(32)     NOT NULL DEFAULT ''
    COMMENT 'ledger.State 值快照（发现时采样）',

  is_confirmed      TINYINT(1)      NOT NULL DEFAULT 0
    COMMENT '是否经双人授权确认为有效残留（1=确认，0=待确认）',

  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '发现时间',

  KEY idx_sr_residue_items_run_id (residue_run_id),
  KEY idx_sr_residue_items_idem_key (idem_key),
  KEY idx_sr_residue_items_confirmed (is_confirmed)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='残留科目明细（每条对应 safesaga.Partial.Done 中一个步骤的 ledger hop）';

-- ─────────────────────────────────────────────────────────────────────────────
-- sr_residue_reports: 扫描报告
-- ─────────────────────────────────────────────────────────────────────────────
-- 每次 Detect 产出一份报告；同一 SagaRunID 可有多份（最新一份作为 Plan 输入）。
-- scan_epoch 单调递增，从 time.Now().UnixNano() 生成。

CREATE TABLE IF NOT EXISTS sr_residue_reports (
  id                VARCHAR(64)     NOT NULL PRIMARY KEY
    COMMENT 'UUID',

  saga_run_id       VARCHAR(64)     NOT NULL
    COMMENT '扫描的 safesaga.Run.ID',

  scan_epoch        BIGINT          NOT NULL DEFAULT 0
    COMMENT '单调递增扫描序号（UnixNano）',

  items_json        MEDIUMTEXT      NOT NULL DEFAULT '[]'
    COMMENT '残留科目列表（JSON 序列化，ResidueItem[]）',

  total_amount      DECIMAL(36,18)  NOT NULL DEFAULT 0
    COMMENT '各 item 金额总和',

  severity          VARCHAR(16)     NOT NULL DEFAULT 'low'
    COMMENT '严重级别',

  scan_note         VARCHAR(2048)   NOT NULL DEFAULT ''
    COMMENT '扫描诊断信息',

  scanned_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '扫描时间',

  KEY idx_sr_residue_reports_saga_run_id_epoch (saga_run_id, scan_epoch)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='残留扫描报告（每次 DetectResidue 产出）';

-- ─────────────────────────────────────────────────────────────────────────────
-- sr_closeout_plans: 强制收口计划
-- ─────────────────────────────────────────────────────────────────────────────
-- 一个 ResidueRun 至多一份活跃 Plan（state IN ('closing','residue_open')）。
-- generation 须与 sr_mutex_gens.generation 一致（启动 Plan 时持有锁的世代）。

CREATE TABLE IF NOT EXISTS sr_closeout_plans (
  id                VARCHAR(64)     NOT NULL PRIMARY KEY
    COMMENT 'UUID',

  residue_run_id    VARCHAR(64)     NOT NULL
    COMMENT '所属 ResidueRun.ID',

  saga_run_id       VARCHAR(64)     NOT NULL
    COMMENT 'safesaga.Run.ID（冗余，便于关联查询）',

  idem_prefix       VARCHAR(256)    NOT NULL
    COMMENT '收口腿幂等键前缀（所有腿的 idem_key = idem_prefix + "-" + leg.idem_suffix）',

  authorizer1       VARCHAR(128)    NOT NULL DEFAULT ''
    COMMENT '双人授权第一人（发起人）',

  authorizer2       VARCHAR(128)    NOT NULL DEFAULT ''
    COMMENT '双人授权第二人（复核人，不得与 authorizer1 相同）',

  generation        BIGINT          NOT NULL DEFAULT 0
    COMMENT '持锁时的世代号（与 sr_mutex_gens.generation 对齐）',

  total_legs        INT             NOT NULL DEFAULT 0
    COMMENT '总腿数',

  applied_legs      INT             NOT NULL DEFAULT 0
    COMMENT '已 applied 腿数',

  failed_legs       INT             NOT NULL DEFAULT 0
    COMMENT '已 failed 腿数',

  skipped_legs      INT             NOT NULL DEFAULT 0
    COMMENT '已 skipped 腿数',

  state             VARCHAR(32)     NOT NULL DEFAULT 'closing'
    COMMENT 'Plan 状态：closing|closed|escalated',

  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '创建时间',

  updated_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '最后更新时间',

  KEY idx_sr_closeout_plans_residue_run_id (residue_run_id),
  KEY idx_sr_closeout_plans_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='强制收口计划';

-- ─────────────────────────────────────────────────────────────────────────────
-- sr_closeout_legs: 收口腿明细
-- ─────────────────────────────────────────────────────────────────────────────
-- Leg FSM：pending → applied / failed / skipped
-- idem_suffix 与 CloseoutPlan.idem_prefix 拼接为全局唯一幂等键。
-- retry_count 超过 MaxLegRetry(5) 后自动标 failed，须人工介入。

CREATE TABLE IF NOT EXISTS sr_closeout_legs (
  id                VARCHAR(64)     NOT NULL PRIMARY KEY
    COMMENT 'UUID',

  plan_id           VARCHAR(64)     NOT NULL
    COMMENT '所属 CloseoutPlan.ID',

  residue_run_id    VARCHAR(64)     NOT NULL
    COMMENT '所属 ResidueRun.ID（冗余，便于关联查询）',

  seq_no            INT             NOT NULL DEFAULT 1
    COMMENT '执行顺序（从 1 开始，严格按序执行）',

  kind              VARCHAR(16)     NOT NULL DEFAULT 'reverse'
    COMMENT '收口腿类型：reverse（冲正）| suspense（挂账）| fence_block（挡写封印）',

  step              VARCHAR(32)     NOT NULL DEFAULT ''
    COMMENT '对应的 saga Step 名称',

  idem_suffix       VARCHAR(256)    NOT NULL
    COMMENT '幂等键后缀（与 plan.idem_prefix 拼接为完整幂等键）',

  amount            DECIMAL(36,18)  NOT NULL DEFAULT 0
    COMMENT '操作金额（fence_block 腿为 0）',

  asset             VARCHAR(32)     NOT NULL DEFAULT ''
    COMMENT '资产标识',

  direction         VARCHAR(64)     NOT NULL DEFAULT ''
    COMMENT '划转方向（fence_block 腿为空）',

  state             VARCHAR(16)     NOT NULL DEFAULT 'pending'
    COMMENT 'Leg 状态：pending|applied|failed|skipped',

  ledger_entry_id   VARCHAR(64)     NULL
    COMMENT '执行后写入的 ledger.Transfer.ID（fence_block 腿为空）',

  fail_reason       VARCHAR(1024)   NULL
    COMMENT '失败原因',

  skip_reason       VARCHAR(1024)   NULL
    COMMENT '跳过原因（须双人授权）',

  retry_count       INT             NOT NULL DEFAULT 0
    COMMENT '已重试次数（达 MaxLegRetry=5 后标 failed）',

  applied_at        DATETIME(3)     NULL
    COMMENT '成功执行时间',

  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '创建时间',

  updated_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '最后更新时间',

  KEY idx_sr_closeout_legs_plan_id_seq (plan_id, seq_no),
  KEY idx_sr_closeout_legs_residue_run_id (residue_run_id),
  KEY idx_sr_closeout_legs_state (state),
  UNIQUE KEY uk_sr_closeout_legs_idem (plan_id, idem_suffix)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='收口腿明细（冲正 / 挂账 / 挡写封印）';

-- ─────────────────────────────────────────────────────────────────────────────
-- sr_close_proofs: 收口终态证明
-- ─────────────────────────────────────────────────────────────────────────────
-- ProofDigest = SHA256(IntentHash + ":" + ResidueHash + ":" + LegHash)
-- is_valid 由最近一次 VerifyProof 校验更新。
-- 证明缺失（无对应行）或 is_valid=0 时，禁止将 ResidueRun 标为 closed。

CREATE TABLE IF NOT EXISTS sr_close_proofs (
  id                VARCHAR(64)     NOT NULL PRIMARY KEY
    COMMENT 'UUID',

  residue_run_id    VARCHAR(64)     NOT NULL
    COMMENT '所属 ResidueRun.ID',

  saga_run_id       VARCHAR(64)     NOT NULL
    COMMENT 'safesaga.Run.ID（冗余）',

  plan_id           VARCHAR(64)     NOT NULL DEFAULT ''
    COMMENT '关联的 CloseoutPlan.ID',

  intent_hash       CHAR(64)        NOT NULL DEFAULT ''
    COMMENT 'SHA256(sorted JSON of Partial.Done steps)',

  residue_hash      CHAR(64)        NOT NULL DEFAULT ''
    COMMENT 'SHA256(sorted JSON of confirmed ResidueItems)',

  leg_hash          CHAR(64)        NOT NULL DEFAULT ''
    COMMENT 'SHA256(sorted JSON of applied CloseoutLegs)',

  proof_digest      CHAR(64)        NOT NULL DEFAULT ''
    COMMENT 'SHA256(IntentHash + ":" + ResidueHash + ":" + LegHash)',

  is_valid          TINYINT(1)      NOT NULL DEFAULT 1
    COMMENT '最近一次校验是否通过（0=失败，需 P1 告警）',

  verified_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '最近一次校验时间',

  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '生成时间',

  UNIQUE KEY uk_sr_close_proofs_residue_run_id (residue_run_id),
  KEY idx_sr_close_proofs_is_valid (is_valid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='收口终态证明（签发后方可标 ResidueRun 为 closed）';

-- ─────────────────────────────────────────────────────────────────────────────
-- sr_mutex_gens: 补偿/收口互斥世代锁
-- ─────────────────────────────────────────────────────────────────────────────
-- 持锁协议（应用层 CAS）：
--   Acquire: INSERT or UPDATE generation++, status='locked', expires_at=NOW()+TTL
--            WHERE status IN ('open','expired','released')
--   Refresh: UPDATE expires_at=NOW()+TTL WHERE saga_run_id=? AND generation=? AND holder=?
--   Release: UPDATE status='released' WHERE saga_run_id=? AND generation=? AND holder=?
--   Reclaim: UPDATE status='expired' WHERE status='locked' AND expires_at < NOW()
--
-- CheckCompensateAllowed：SELECT status, expires_at WHERE saga_run_id=?
--   → status='locked' AND expires_at>NOW() → 拒绝 Compensate（ErrCompensateBlock）

CREATE TABLE IF NOT EXISTS sr_mutex_gens (
  id                VARCHAR(64)     NOT NULL PRIMARY KEY
    COMMENT 'UUID（首次 Acquire 时生成）',

  saga_run_id       VARCHAR(64)     NOT NULL
    COMMENT '锁绑定的 safesaga.Run.ID；唯一约束保证单 Run 单锁',

  generation        BIGINT          NOT NULL DEFAULT 1
    COMMENT '世代号（从 1 开始，每次 Acquire 原子递增）；ErrStaleGen 依赖此字段',

  holder            VARCHAR(255)    NOT NULL DEFAULT ''
    COMMENT '持锁 worker 标识（hostname:pid:uuid）',

  status            VARCHAR(16)     NOT NULL DEFAULT 'open'
    COMMENT '锁状态：open|locked|expired|released',

  expires_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT 'locked 状态有效期（过期后 MutexReclaimer 标为 expired）',

  locked_at         DATETIME(3)     NULL
    COMMENT '最近一次 Acquire 时间',

  released_at       DATETIME(3)     NULL
    COMMENT '最近一次 Release 时间',

  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '首次创建时间',

  updated_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '最后更新时间',

  UNIQUE KEY uk_sr_mutex_gens_saga_run_id (saga_run_id),
  KEY idx_sr_mutex_gens_status_expires (status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='补偿/收口互斥世代锁（防双 worker 并行 Compensate/Closeout）';

-- ─────────────────────────────────────────────────────────────────────────────
-- sr_idem_blocks: 幂等拒写记录
-- ─────────────────────────────────────────────────────────────────────────────
-- FenceBlock 腿执行后写入此表；
-- 之后同一 idem_key 的 saga compensate 续写请求须在入口检查此表，返回拒绝。
-- 唯一约束保证幂等：重复写入 ON DUPLICATE KEY 忽略即可。

CREATE TABLE IF NOT EXISTS sr_idem_blocks (
  idem_key          VARCHAR(256)    NOT NULL PRIMARY KEY
    COMMENT '被封堵的幂等键（与 ledger 幂等键命名空间对齐）',

  reason            VARCHAR(1024)   NOT NULL DEFAULT ''
    COMMENT '封堵原因（saga_run_id + plan_id + leg_id）',

  blocked_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
    COMMENT '封堵时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='幂等拒写记录（FenceBlock 腿写入，阻止 saga 续写相同幂等键）';
