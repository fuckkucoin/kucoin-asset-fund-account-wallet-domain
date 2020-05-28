-- 入金二次认领与自动贷记互斥封堵引擎持久化表。
-- 模块：internal/claimmutex；任务 D · 第十二轮。
--
-- 作者：Axel
-- 表清单：
--   dc_output_leases  — 输出效果世代租约
--   dc_dup_cases      — 同输出双贷案件
--   dc_auto_binds     — 自动贷记绑定记录
--   dc_effect_proofs  — 认领效果证明

-- ─── dc_output_leases ────────────────────────────────────────────────────────
-- 唯一约束 (chain, tx_hash, output_index) 是互斥 CAS 的核心依赖：
-- 并发两条 Claim/auto-credit 只有一条 INSERT 成功，另一条得 ErrMutexConflict。

CREATE TABLE IF NOT EXISTS dc_output_leases (
  lease_id       VARCHAR(64)  NOT NULL PRIMARY KEY,
  chain          VARCHAR(32)  NOT NULL,
  tx_hash        VARCHAR(128) NOT NULL,
  output_index   INT          NOT NULL DEFAULT 0,
  generation     BIGINT       NOT NULL,
  state          VARCHAR(20)  NOT NULL DEFAULT 'open',
                              -- open | claimed | auto_credited | blocked | expired
  winner_kind    VARCHAR(20)  NOT NULL DEFAULT '',
                              -- claim | auto_credit
  winner_actor   VARCHAR(64)  NOT NULL DEFAULT '',
  claim_case_id  VARCHAR(64)  NULL,
  issued_at      DATETIME(3)  NOT NULL,
  expires_at     DATETIME(3)  NOT NULL,
  claimed_at     DATETIME(3)  NULL,
  blocked_at     DATETIME(3)  NULL,
  block_reason   TEXT         NULL,
  UNIQUE KEY uk_dc_lease_output (chain, tx_hash, output_index),
  KEY idx_dc_lease_state   (state, expires_at),
  KEY idx_dc_lease_actor   (winner_actor, state)
);

-- ─── dc_dup_cases ────────────────────────────────────────────────────────────
-- 同输出双贷案件。FSM: open → proven → closed。

CREATE TABLE IF NOT EXISTS dc_dup_cases (
  case_id        VARCHAR(64)  NOT NULL PRIMARY KEY,
  chain          VARCHAR(32)  NOT NULL,
  tx_hash        VARCHAR(128) NOT NULL,
  output_index   INT          NOT NULL DEFAULT 0,
  lease_id       VARCHAR(64)  NOT NULL DEFAULT '',
  first_kind     VARCHAR(20)  NOT NULL DEFAULT '',
  first_actor    VARCHAR(64)  NOT NULL DEFAULT '',
  first_idem     VARCHAR(128) NULL,
  second_kind    VARCHAR(20)  NOT NULL DEFAULT '',
  second_actor   VARCHAR(64)  NOT NULL DEFAULT '',
  second_idem    VARCHAR(128) NULL,
  delta_amount   VARCHAR(64)  NOT NULL DEFAULT '0',
  state          VARCHAR(16)  NOT NULL DEFAULT 'open',
                              -- open | proven | closed
  detected_at    DATETIME(3)  NOT NULL,
  proven_at      DATETIME(3)  NULL,
  closed_at      DATETIME(3)  NULL,
  actor1         VARCHAR(64)  NULL,
  actor2         VARCHAR(64)  NULL,
  close_note     TEXT         NULL,
  KEY idx_dc_cases_output (chain, tx_hash, output_index, state),
  KEY idx_dc_cases_state  (state, detected_at)
);

-- ─── dc_auto_binds ───────────────────────────────────────────────────────────
-- 自动贷记绑定记录。FSM: pending → bound | rejected → released。

CREATE TABLE IF NOT EXISTS dc_auto_binds (
  bind_id        VARCHAR(64)  NOT NULL PRIMARY KEY,
  chain          VARCHAR(32)  NOT NULL,
  tx_hash        VARCHAR(128) NOT NULL,
  output_index   INT          NOT NULL DEFAULT 0,
  generation     BIGINT       NOT NULL DEFAULT 0,
  lease_id       VARCHAR(64)  NOT NULL DEFAULT '',
  user_id        VARCHAR(64)  NOT NULL DEFAULT '',
  asset          VARCHAR(16)  NOT NULL DEFAULT '',
  amount         VARCHAR(64)  NOT NULL DEFAULT '0',
  idem_key       VARCHAR(128) NOT NULL DEFAULT '',
  state          VARCHAR(16)  NOT NULL DEFAULT 'pending',
                              -- pending | bound | rejected | released
  bound_at       DATETIME(3)  NOT NULL,
  resolved_at    DATETIME(3)  NULL,
  reject_note    TEXT         NULL,
  KEY idx_dc_binds_output (chain, tx_hash, output_index, state),
  KEY idx_dc_binds_user   (user_id, state)
);

-- ─── dc_effect_proofs ────────────────────────────────────────────────────────
-- 认领效果证明。FSM: pending → verified → sealed | failed。
-- 认领标记为完成前须有 verified/sealed 证明，否则返回 ErrProofMissing。

CREATE TABLE IF NOT EXISTS dc_effect_proofs (
  proof_id       VARCHAR(64)  NOT NULL PRIMARY KEY,
  chain          VARCHAR(32)  NOT NULL,
  tx_hash        VARCHAR(128) NOT NULL,
  output_index   INT          NOT NULL DEFAULT 0,
  lease_id       VARCHAR(64)  NOT NULL DEFAULT '',
  generation     BIGINT       NOT NULL DEFAULT 0,
  claim_kind     VARCHAR(20)  NOT NULL DEFAULT '',
                              -- claim | auto_credit
  actor_id       VARCHAR(64)  NOT NULL DEFAULT '',
  user_id        VARCHAR(64)  NOT NULL DEFAULT '',
  asset          VARCHAR(16)  NOT NULL DEFAULT '',
  amount         VARCHAR(64)  NOT NULL DEFAULT '0',
  idem_key       VARCHAR(128) NOT NULL DEFAULT '',
  state          VARCHAR(16)  NOT NULL DEFAULT 'pending',
                              -- pending | verified | sealed | failed
  created_at     DATETIME(3)  NOT NULL,
  verified_at    DATETIME(3)  NULL,
  sealed_at      DATETIME(3)  NULL,
  fail_reason    TEXT         NULL,
  sealed_by      VARCHAR(128) NULL,
  KEY idx_dc_proofs_output (chain, tx_hash, output_index),
  KEY idx_dc_proofs_state  (state, created_at),
  KEY idx_dc_proofs_lease  (lease_id)
);
