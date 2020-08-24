-- 定投追赶超额借记与取消竞态封堵引擎——持久化表。
-- 模块：recguard（internal/recguard）
-- 关联：rec_plans（既有 recbuy 表，只读外键）
--
-- 四张表：
--   rec_plan_gens      — 计划世代令牌（单调递增，取消抬代）
--   catchup_windows    — 单轮追赶会计窗口（cap + 累计）
--   half_debit_residues — 半扣残留（回滚/挂账收口）
--   debit_proofs       — 追赶终态证明（Write-Once）
--
-- 不变式（应用层保证，DDL 辅助约束）：
--   rec_plan_gens: (plan_id, generation) 唯一；generation > 0。
--   catchup_windows: (plan_id, generation) 最多一个 state<>'closed'。
--   debit_proofs: proof_hash 唯一（Write-Once）。
--
-- 作者：Span / Likeo / King
-- 日期：2022.07.17

-- ─────────────────────────────────────────────────────────────
-- 1. rec_plan_gens — 计划世代令牌
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS rec_plan_gens (
  id            VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id       VARCHAR(64)  NOT NULL COMMENT '关联 recbuy 计划 ID',
  generation    BIGINT       NOT NULL COMMENT '世代号，单调递增，初始为 1',
  reason        VARCHAR(32)  NOT NULL DEFAULT 'initial' COMMENT 'initial|cancel|reopen',
  actor         VARCHAR(128) NOT NULL DEFAULT '' COMMENT '操作者标识',
  created_at    DATETIME(3)  NOT NULL,
  expires_at    DATETIME(3)  NULL COMMENT '零/NULL 表示永不过期',
  UNIQUE KEY uk_rec_plan_gen (plan_id, generation),
  KEY idx_rec_plan_gen_plan (plan_id),
  KEY idx_rec_plan_gen_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='定投计划世代令牌；取消操作抬代，旧世代 tick 请求自动失效。';

-- ─────────────────────────────────────────────────────────────
-- 2. catchup_windows — 单轮追赶会计窗口
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS catchup_windows (
  id            VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id       VARCHAR(64)  NOT NULL,
  generation    BIGINT       NOT NULL COMMENT '窗口绑定的世代号',
  cap           DECIMAL(36,18) NOT NULL DEFAULT 0 COMMENT '本轮最大可借记额（quote）',
  per_tick      DECIMAL(36,18) NOT NULL DEFAULT 0 COMMENT '单次 tick 金额',
  debited       DECIMAL(36,18) NOT NULL DEFAULT 0 COMMENT '已确认借记累计',
  tick_count    INT          NOT NULL DEFAULT 0,
  filled_count  INT          NOT NULL DEFAULT 0,
  skip_count    INT          NOT NULL DEFAULT 0,
  half_count    INT          NOT NULL DEFAULT 0,
  state         VARCHAR(16)  NOT NULL DEFAULT 'open' COMMENT 'open|capped|closed',
  proof_id      VARCHAR(64)  NULL COMMENT 'state=closed 后填充',
  opened_at     DATETIME(3)  NOT NULL,
  closed_at     DATETIME(3)  NULL,
  KEY idx_catchup_win_plan (plan_id),
  KEY idx_catchup_win_plan_gen (plan_id, generation),
  KEY idx_catchup_win_state (state),
  CONSTRAINT chk_catchup_win_state CHECK (state IN ('open','capped','closed')),
  CONSTRAINT chk_catchup_win_debited CHECK (debited >= 0),
  CONSTRAINT chk_catchup_win_cap CHECK (cap >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='单轮追赶借记会计窗口；debited<=cap；closed 时 proof_id 非空。';

-- ─────────────────────────────────────────────────────────────
-- 3. half_debit_residues — 半扣残留
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS half_debit_residues (
  id                   VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id              VARCHAR(64)  NOT NULL,
  window_id            VARCHAR(64)  NULL,
  tick_at              DATETIME(3)  NOT NULL COMMENT '对应 tick 的计划时间点',
  generation           BIGINT       NOT NULL,
  original_amount      DECIMAL(36,18) NOT NULL DEFAULT 0 COMMENT '意图借记额',
  residue_amount       DECIMAL(36,18) NOT NULL DEFAULT 0 COMMENT '实际被扣额',
  ledger_entry_id      VARCHAR(128) NOT NULL DEFAULT '' COMMENT '已创建账本行 ID',
  state                VARCHAR(16)  NOT NULL DEFAULT 'open' COMMENT 'open|rolled_back|hung',
  cause                VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '产生原因',
  rollback_ledger_id   VARCHAR(128) NOT NULL DEFAULT '' COMMENT '回滚冲销账本行 ID',
  hung_reason          VARCHAR(512) NOT NULL DEFAULT '',
  detected_at          DATETIME(3)  NOT NULL,
  closed_at            DATETIME(3)  NULL,
  KEY idx_half_debit_plan (plan_id),
  KEY idx_half_debit_plan_state (plan_id, state),
  KEY idx_half_debit_window (window_id),
  KEY idx_half_debit_detected (detected_at),
  CONSTRAINT chk_half_debit_state CHECK (state IN ('open','rolled_back','hung')),
  CONSTRAINT chk_half_debit_residue CHECK (residue_amount >= 0),
  CONSTRAINT chk_half_debit_original CHECK (original_amount >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='追赶途中半扣残留凭据；state=rolled_back 时 rollback_ledger_id 非空；state=hung 时 hung_reason 非空。';

-- ─────────────────────────────────────────────────────────────
-- 4. debit_proofs — 追赶终态证明（Write-Once）
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS debit_proofs (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id         VARCHAR(64)  NOT NULL,
  window_id       VARCHAR(64)  NOT NULL,
  generation      BIGINT       NOT NULL,
  intended_ticks  INT          NOT NULL DEFAULT 0,
  filled_ticks    INT          NOT NULL DEFAULT 0,
  skip_ticks      INT          NOT NULL DEFAULT 0,
  half_debit_count INT         NOT NULL DEFAULT 0,
  half_debit_ids  JSON         NULL COMMENT '半扣残留 ID 列表',
  total_debited   DECIMAL(36,18) NOT NULL DEFAULT 0,
  cap_amount      DECIMAL(36,18) NOT NULL DEFAULT 0,
  proof_hash      VARCHAR(128) NOT NULL COMMENT 'sha256(plan|window|gen|debited|intended|filled|skip)',
  created_at      DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_debit_proof_hash (proof_hash),
  UNIQUE KEY uk_debit_proof_window (window_id),
  KEY idx_debit_proof_plan (plan_id),
  KEY idx_debit_proof_created (created_at),
  CONSTRAINT chk_debit_proof_ticks CHECK (filled_ticks + skip_ticks = intended_ticks OR intended_ticks = 0),
  CONSTRAINT chk_debit_proof_debited CHECK (total_debited >= 0),
  CONSTRAINT chk_debit_proof_cap CHECK (cap_amount >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='单轮追赶终态证明（Write-Once）；proof_hash 唯一；filled+skip==intended。';

-- ─────────────────────────────────────────────────────────────
-- 5. guard_tick_records — Tick 守护记录（可选，供审计）
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS guard_tick_records (
  id            VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id       VARCHAR(64)  NOT NULL,
  window_id     VARCHAR(64)  NULL,
  generation    BIGINT       NOT NULL,
  tick_at       DATETIME(3)  NOT NULL,
  state         VARCHAR(16)  NOT NULL DEFAULT 'due' COMMENT 'due|debiting|filled|skipped|reverted',
  skip_cause    VARCHAR(32)  NOT NULL DEFAULT '',
  amount        DECIMAL(36,18) NOT NULL DEFAULT 0,
  debited_amt   DECIMAL(36,18) NOT NULL DEFAULT 0,
  ledger_id     VARCHAR(128) NOT NULL DEFAULT '',
  residue_id    VARCHAR(64)  NULL,
  started_at    DATETIME(3)  NOT NULL,
  finished_at   DATETIME(3)  NULL,
  KEY idx_guard_tick_plan (plan_id),
  KEY idx_guard_tick_window (window_id),
  KEY idx_guard_tick_state (state),
  KEY idx_guard_tick_tick_at (tick_at),
  CONSTRAINT chk_guard_tick_state CHECK (state IN ('due','debiting','filled','skipped','reverted'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='单次 tick 守护层生命周期记录（审计用）。';
