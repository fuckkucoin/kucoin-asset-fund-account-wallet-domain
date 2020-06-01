-- 资金账户生命周期冻结关闭与并发 hop 竞态封堵引擎持久化表。
-- 模块：internal/acctlifegate；任务 D · 第十三轮。
--
-- 作者：Kinsley
-- 表清单：
--   account_life_generations  — per-(user,book) 生命周期世代记录
--   account_life_leases       — 生命周期世代租约
--   account_move_binds        — 在途 Move 绑定（互斥屏障）
--   account_life_drift_cases  — 半开漂移案件与挡写证明

-- ─── account_life_generations ────────────────────────────────────────────────
-- 单调递增，不删旧行，以便重放和审计。
-- 唯一约束 (user_id, book, generation) 防止 BumpGeneration 重复写。

CREATE TABLE IF NOT EXISTS account_life_generations (
  gen_id       VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id      VARCHAR(64)  NOT NULL,
  book         VARCHAR(32)  NOT NULL,
  generation   BIGINT       NOT NULL,
  action       VARCHAR(20)  NOT NULL DEFAULT '',
               -- open | freeze | unfreeze | close
  actor        VARCHAR(64)  NOT NULL DEFAULT '',
  created_at   DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_alg_user_book_gen (user_id, book, generation),
  KEY idx_alg_user_book_latest    (user_id, book, generation DESC)
);

-- ─── account_life_leases ─────────────────────────────────────────────────────
-- GateMove 调用前由 AcquireLease 发放；携带 generation；
-- Freeze/Close 成功后通过 RevokeOpenLeases 批量吊销。
-- 状态机：open → used | expired | revoked

CREATE TABLE IF NOT EXISTS account_life_leases (
  lease_id     VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id      VARCHAR(64)  NOT NULL,
  book         VARCHAR(32)  NOT NULL,
  generation   BIGINT       NOT NULL,
  state        VARCHAR(16)  NOT NULL DEFAULT 'open',
               -- open | used | expired | revoked
  issued_at    DATETIME(3)  NOT NULL,
  expires_at   DATETIME(3)  NOT NULL,
  used_at      DATETIME(3)  NULL,
  expired_at   DATETIME(3)  NULL,
  revoked_at   DATETIME(3)  NULL,
  revoke_note  TEXT         NULL,
  KEY idx_all_user_book_state   (user_id, book, state),
  KEY idx_all_state_expires     (state, expires_at),
  KEY idx_all_gen               (user_id, book, generation)
);

-- ─── account_move_binds ──────────────────────────────────────────────────────
-- 在途 Move 互斥屏障。pending 条目存在时 Freeze/Close 被阻止。
-- 状态机：pending → completed | compensated | timed_out
--
-- idem_key 唯一约束保证幂等：同一 GateMove 调用重试不产生重复绑定。

CREATE TABLE IF NOT EXISTS account_move_binds (
  bind_id      VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id      VARCHAR(64)  NOT NULL,
  from_book    VARCHAR(32)  NOT NULL,
  to_book      VARCHAR(32)  NOT NULL,
  asset        VARCHAR(16)  NOT NULL DEFAULT '',
  amount       VARCHAR(64)  NOT NULL DEFAULT '0',
  lease_id     VARCHAR(64)  NOT NULL DEFAULT '',
  generation   BIGINT       NOT NULL DEFAULT 0,
  idem_key     VARCHAR(128) NOT NULL DEFAULT '',
  state        VARCHAR(16)  NOT NULL DEFAULT 'pending',
               -- pending | completed | compensated | timed_out
  bound_at     DATETIME(3)  NOT NULL,
  expires_at   DATETIME(3)  NOT NULL,
  resolved_at  DATETIME(3)  NULL,
  note         TEXT         NULL,
  UNIQUE KEY uk_amb_idem   (idem_key),
  KEY idx_amb_user_book    (user_id, from_book, state),
  KEY idx_amb_state_exp    (state, expires_at),
  KEY idx_amb_lease        (lease_id)
);

-- ─── account_life_drift_cases ────────────────────────────────────────────────
-- 半开漂移案件。CheckHop 时科目 active → Move 时已 frozen/closed。
-- open/proven 状态下挡同 (user_id, book) 的借记操作。
-- 状态机：open → proven → closed
--
-- 证明字段说明：
--   status_at_check  : CheckHop 时的科目状态（预期为 active）
--   status_at_move   : Move 执行时的科目状态（frozen 或 closed）
--   life_gen_at_check: CheckHop 时的 life_gen
--   life_gen_at_move : Move 时的 life_gen（应 > life_gen_at_check）
--   actor1/actor2    : ProveDrift 双人确认签名（两者不得相同）

CREATE TABLE IF NOT EXISTS account_life_drift_cases (
  case_id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id           VARCHAR(64)  NOT NULL,
  book              VARCHAR(32)  NOT NULL,
  detected_at       DATETIME(3)  NOT NULL,
  check_hop_at      DATETIME(3)  NOT NULL,
  move_attempt_at   DATETIME(3)  NOT NULL,
  status_at_check   VARCHAR(16)  NOT NULL DEFAULT '',
  status_at_move    VARCHAR(16)  NOT NULL DEFAULT '',
  lease_id          VARCHAR(64)  NOT NULL DEFAULT '',
  move_idem_key     VARCHAR(128) NOT NULL DEFAULT '',
  life_gen_at_check BIGINT       NOT NULL DEFAULT 0,
  life_gen_at_move  BIGINT       NOT NULL DEFAULT 0,
  state             VARCHAR(16)  NOT NULL DEFAULT 'open',
               -- open | proven | closed
  blocked_at        DATETIME(3)  NULL,
  proven_at         DATETIME(3)  NULL,
  closed_at         DATETIME(3)  NULL,
  actor1            VARCHAR(64)  NULL,
  actor2            VARCHAR(64)  NULL,
  proof_note        TEXT         NULL,
  close_note        TEXT         NULL,
  KEY idx_aldc_user_book_state  (user_id, book, state),
  KEY idx_aldc_state_detected   (state, detected_at),
  KEY idx_aldc_lease            (lease_id)
);
