-- 红包过期回收与迟领双贷竞态编排数据表。
-- 功能：支撑 envreclaim 四大能力：
--   1. EnvelopeLife FSM（信封存活状态机）
--   2. ClaimToken 互斥令牌（claim vs reclaim 竞态保护）
--   3. ReclaimJob 后台回收作业
--   4. DoubleCreditProof 双贷证明 + AgingEvent 老化事件
--
-- 边界：
--   仅写 envelope_ / claim_ / reclaim_ / double_credit_ 前缀表。
--   不修改 ledger_entries / holdcat / clearing_* 等核心账本表。
--   不存储用户余额或资金流水，只存状态机控制信息。
--
-- 不变量：
--   envelope_lives.state         枚举须与 envreclaim.LifeState 对齐。
--   claim_tokens.state           枚举须与 envreclaim.TokenState 对齐。
--   reclaim_jobs.state           枚举须与 envreclaim.JobState 对齐。
--   double_credit_proofs.state   枚举须与 envreclaim.ProofState 对齐。
--   aging_events 追加只写，生成后不可删除或覆盖。
--   每个 envelope_id 最多一条 committed claim_token（唯一约束保障）。
--
-- 作者：WuLiang / ChenRui
-- 日期：2022.08.15

-- ─────────────────────────────────────────────────────────────────────────────
-- envelope_lives 信封生命周期表
-- state: placed → open → [claiming | reclaiming] → closed
-- close_reason: fully_claimed | expired_reclaim | force_reclaim
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS envelope_lives (
  id               VARCHAR(64)     NOT NULL,
  envelope_id      VARCHAR(128)    NOT NULL,
  sender_id        VARCHAR(64)     NOT NULL,
  asset            VARCHAR(16)     NOT NULL,
  total_amount     DECIMAL(36,18)  NOT NULL,
  state            VARCHAR(24)     NOT NULL DEFAULT 'placed',
  close_reason     VARCHAR(32)     NOT NULL DEFAULT '',
  expires_at       DATETIME(3)     NOT NULL,
  open_at          DATETIME(3)     NULL,
  closed_at        DATETIME(3)     NULL,
  age_sla_sec      BIGINT          NOT NULL DEFAULT 259200,
  escalated_at     DATETIME(3)     NULL,
  escalated_desk   VARCHAR(64)     NOT NULL DEFAULT '',
  created_at       DATETIME(3)     NOT NULL,
  updated_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_envelope_lives_env_id (envelope_id),
  KEY idx_envelope_lives_state        (state, expires_at),
  KEY idx_envelope_lives_sender       (sender_id, state),
  KEY idx_envelope_lives_expires      (expires_at, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='信封存活状态机记录';

-- ─────────────────────────────────────────────────────────────────────────────
-- claim_tokens 互斥令牌表
-- kind:  claim | reclaim
-- state: reserved → committed | aborted
-- 每个 envelope_id 最多一条 committed 令牌（唯一约束）。
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS claim_tokens (
  id               VARCHAR(64)     NOT NULL,
  envelope_id      VARCHAR(128)    NOT NULL,
  life_id          VARCHAR(64)     NOT NULL,
  kind             VARCHAR(16)     NOT NULL,
  holder_id        VARCHAR(64)     NOT NULL,
  state            VARCHAR(16)     NOT NULL DEFAULT 'reserved',
  amount           DECIMAL(36,18)  NOT NULL,
  ledger_idem_key  VARCHAR(256)    NOT NULL DEFAULT '',
  committed_at     DATETIME(3)     NULL,
  aborted_at       DATETIME(3)     NULL,
  abort_reason     VARCHAR(64)     NOT NULL DEFAULT '',
  created_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  -- 每个信封只允许一条 committed token（互斥保障）。
  UNIQUE KEY uk_claim_tokens_committed (envelope_id, state),
  KEY idx_claim_tokens_envelope    (envelope_id, kind),
  KEY idx_claim_tokens_life        (life_id),
  KEY idx_claim_tokens_holder      (holder_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='claim/reclaim 互斥令牌';

-- ─────────────────────────────────────────────────────────────────────────────
-- reclaim_jobs 回收作业表
-- state: pending → in_flight → done | failed | cancelled
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS reclaim_jobs (
  id               VARCHAR(64)     NOT NULL,
  envelope_id      VARCHAR(128)    NOT NULL,
  life_id          VARCHAR(64)     NOT NULL,
  sender_id        VARCHAR(64)     NOT NULL,
  asset            VARCHAR(16)     NOT NULL,
  amount           DECIMAL(36,18)  NOT NULL,
  state            VARCHAR(16)     NOT NULL DEFAULT 'pending',
  scheduled_at     DATETIME(3)     NOT NULL,
  started_at       DATETIME(3)     NULL,
  finished_at      DATETIME(3)     NULL,
  retry_count      INT             NOT NULL DEFAULT 0,
  max_retry        INT             NOT NULL DEFAULT 5,
  last_error       VARCHAR(512)    NOT NULL DEFAULT '',
  token_id         VARCHAR(64)     NOT NULL DEFAULT '',
  ledger_entry_id  VARCHAR(64)     NOT NULL DEFAULT '',
  created_at       DATETIME(3)     NOT NULL,
  updated_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_reclaim_jobs_envelope (envelope_id, state),
  KEY idx_reclaim_jobs_scheduled  (state, scheduled_at),
  KEY idx_reclaim_jobs_life       (life_id),
  KEY idx_reclaim_jobs_sender     (sender_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='过期回收后台作业';

-- ─────────────────────────────────────────────────────────────────────────────
-- double_credit_proofs 双贷证明表
-- state: open → reversed | blocked → resolved
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS double_credit_proofs (
  id                    VARCHAR(64)     NOT NULL,
  envelope_id           VARCHAR(128)    NOT NULL,
  life_id               VARCHAR(64)     NOT NULL,
  state                 VARCHAR(16)     NOT NULL DEFAULT 'open',
  -- first_credit 字段（第一条 P2PCredit 摘要）
  first_entry_id        VARCHAR(64)     NOT NULL DEFAULT '',
  first_user_id         VARCHAR(64)     NOT NULL DEFAULT '',
  first_amount          DECIMAL(36,18)  NOT NULL DEFAULT 0,
  first_direction       VARCHAR(32)     NOT NULL DEFAULT '',
  first_idem_key        VARCHAR(256)    NOT NULL DEFAULT '',
  first_recorded_at     DATETIME(3)     NULL,
  -- second_credit 字段（第二条 P2PCredit 摘要）
  second_entry_id       VARCHAR(64)     NOT NULL DEFAULT '',
  second_user_id        VARCHAR(64)     NOT NULL DEFAULT '',
  second_amount         DECIMAL(36,18)  NOT NULL DEFAULT 0,
  second_direction      VARCHAR(32)     NOT NULL DEFAULT '',
  second_idem_key       VARCHAR(256)    NOT NULL DEFAULT '',
  second_recorded_at    DATETIME(3)     NULL,
  -- 处置信息
  reversal_entry_id     VARCHAR(64)     NOT NULL DEFAULT '',
  blocked_place_id      VARCHAR(64)     NOT NULL DEFAULT '',
  detected_at           DATETIME(3)     NOT NULL,
  resolved_at           DATETIME(3)     NULL,
  resolved_by           VARCHAR(64)     NOT NULL DEFAULT '',
  exported_at           DATETIME(3)     NULL,
  created_at            DATETIME(3)     NOT NULL,
  updated_at            DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_dc_proofs_envelope_open (envelope_id, state),
  KEY idx_dc_proofs_state       (state, created_at),
  KEY idx_dc_proofs_life        (life_id),
  KEY idx_dc_proofs_detected    (detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='双 P2PCredit 贷记证明';

-- ─────────────────────────────────────────────────────────────────────────────
-- aging_events 老化告警事件表（追加只写）
-- 不设更新，生成后不可删除。
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aging_events (
  id               VARCHAR(64)     NOT NULL,
  envelope_id      VARCHAR(128)    NOT NULL,
  life_id          VARCHAR(64)     NOT NULL,
  age_seconds      BIGINT          NOT NULL,
  escalate_desk    VARCHAR(64)     NOT NULL DEFAULT '',
  created_at       DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  KEY idx_aging_events_envelope  (envelope_id, created_at),
  KEY idx_aging_events_life      (life_id),
  KEY idx_aging_events_ts        (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='信封老化告警事件（追加只写）';
