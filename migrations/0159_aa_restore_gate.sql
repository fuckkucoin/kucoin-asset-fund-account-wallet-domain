-- 灾备快照回灌世代闸与资金账脑裂封堵——数据库迁移（第 49 版）。
--
-- 新增六张表：
--   restore_plans       RestorePlan 回灌计划聚合根（FSM 状态机）
--   restore_gens        GenGate 世代闸审计记录
--   write_mutexes       WriteMutex 活账写互斥锁
--   blocked_writes      BlockedWrite 被阻断写意图审计日志
--   half_seals          HalfSeal 半恢复封印记录
--   splitbrain_proofs   SplitBrainProof 脑裂证明
--   proof_approvals     ProofApproval 四眼审批条目
--   restore_gen_record  全局世代水位记录（单例行）
--
-- 隔离约束：
--   本迁移不写入、不修改 r7 cfg 双评表（gray_releases / dual_decisions / split_diffs / cfg_seal_actions / cfg_rollback_proofs）。
--   本迁移不写入、不修改 r6 leddiff 相关表。
--   restore_plan_id / splitbrain_proof_id 作为只读外键供 A–G 其他包引用。
--
-- 作者：EliotVoss
-- 日期：2020.12.21

-- ─── 1. restore_plans ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS restore_plans (
  id                VARCHAR(64)   NOT NULL PRIMARY KEY,
  blob_id           VARCHAR(128)  NOT NULL,
  snap_id           VARCHAR(128)  NOT NULL DEFAULT '',
  target_gen        BIGINT        NOT NULL DEFAULT 0,
  live_watermark    BIGINT        NOT NULL DEFAULT 0,
  balance_root_pre  VARCHAR(128)  NOT NULL DEFAULT '',
  balance_root_post VARCHAR(128)  NOT NULL DEFAULT '',
  state             VARCHAR(32)   NOT NULL DEFAULT 'planned',
  creator           VARCHAR(64)   NOT NULL DEFAULT '',
  reason            VARCHAR(512)  NOT NULL DEFAULT '',
  created_at        DATETIME(3)   NOT NULL,
  updated_at        DATETIME(3)   NOT NULL,
  frozen_at         DATETIME(3)   NULL,
  import_start_at   DATETIME(3)   NULL,
  verify_start_at   DATETIME(3)   NULL,
  sealed_at         DATETIME(3)   NULL,
  aborted_at        DATETIME(3)   NULL,
  abort_reason      VARCHAR(512)  NOT NULL DEFAULT '',
  KEY idx_restore_plans_state        (state),
  KEY idx_restore_plans_created_at   (created_at),
  KEY idx_restore_plans_blob_id      (blob_id)
);

-- ─── 2. restore_gens （世代闸审计记录）──────────────────────────────────────
CREATE TABLE IF NOT EXISTS restore_gens (
  id             VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id        VARCHAR(64)  NOT NULL,
  target_gen     BIGINT       NOT NULL DEFAULT 0,
  system_gen     BIGINT       NOT NULL DEFAULT 0,
  live_watermark BIGINT       NOT NULL DEFAULT 0,
  allowed        TINYINT(1)   NOT NULL DEFAULT 0,
  deny_reason    VARCHAR(255) NOT NULL DEFAULT '',
  created_at     DATETIME(3)  NOT NULL,
  KEY idx_restore_gens_plan_id   (plan_id),
  KEY idx_restore_gens_created_at (created_at)
);

-- ─── 3. write_mutexes （活账写互斥锁）──────────────────────────────────────
CREATE TABLE IF NOT EXISTS write_mutexes (
  id          VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id     VARCHAR(64)  NOT NULL,
  state       VARCHAR(32)  NOT NULL DEFAULT 'active',
  holder_gen  BIGINT       NOT NULL DEFAULT 0,
  expires_at  DATETIME(3)  NOT NULL,
  acquired_at DATETIME(3)  NOT NULL,
  released_at DATETIME(3)  NULL,
  created_at  DATETIME(3)  NOT NULL,
  updated_at  DATETIME(3)  NOT NULL,
  KEY idx_write_mutexes_state      (state),
  KEY idx_write_mutexes_plan_id    (plan_id),
  KEY idx_write_mutexes_expires_at (expires_at)
);

-- ─── 4. blocked_writes （被阻断写意图审计）─────────────────────────────────
CREATE TABLE IF NOT EXISTS blocked_writes (
  id         VARCHAR(64)  NOT NULL PRIMARY KEY,
  mutex_id   VARCHAR(64)  NOT NULL,
  plan_id    VARCHAR(64)  NOT NULL,
  kind       VARCHAR(32)  NOT NULL DEFAULT '',
  user_id    VARCHAR(64)  NOT NULL DEFAULT '',
  asset      VARCHAR(16)  NOT NULL DEFAULT '',
  amount     VARCHAR(64)  NOT NULL DEFAULT '',
  reason     VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3)  NOT NULL,
  KEY idx_blocked_writes_mutex_id  (mutex_id),
  KEY idx_blocked_writes_plan_id   (plan_id),
  KEY idx_blocked_writes_created_at (created_at)
);

-- ─── 5. half_seals （半恢复封印）────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS half_seals (
  id          VARCHAR(64)   NOT NULL PRIMARY KEY,
  plan_id     VARCHAR(64)   NOT NULL,
  written_keys TEXT         NOT NULL,         -- JSON 数组：["kind|key",...]
  failed_at   DATETIME(3)   NOT NULL,
  fail_reason VARCHAR(512)  NOT NULL DEFAULT '',
  resumed     TINYINT(1)    NOT NULL DEFAULT 0,
  resumed_at  DATETIME(3)   NULL,
  created_at  DATETIME(3)   NOT NULL,
  updated_at  DATETIME(3)   NOT NULL,
  UNIQUE KEY uk_half_seals_plan_id (plan_id),
  KEY idx_half_seals_resumed (resumed)
);

-- ─── 6. splitbrain_proofs （脑裂证明）──────────────────────────────────────
CREATE TABLE IF NOT EXISTS splitbrain_proofs (
  id                    VARCHAR(64)   NOT NULL PRIMARY KEY,
  plan_id               VARCHAR(64)   NOT NULL,
  balance_root_pre      VARCHAR(128)  NOT NULL DEFAULT '',
  balance_root_post     VARCHAR(128)  NOT NULL DEFAULT '',
  balance_root_target   VARCHAR(128)  NOT NULL DEFAULT '',
  diff_summary          TEXT          NOT NULL,
  state                 VARCHAR(32)   NOT NULL DEFAULT 'open',
  opened_at             DATETIME(3)   NOT NULL,
  cleared_at            DATETIME(3)   NULL,
  created_at            DATETIME(3)   NOT NULL,
  updated_at            DATETIME(3)   NOT NULL,
  KEY idx_splitbrain_proofs_plan_id   (plan_id),
  KEY idx_splitbrain_proofs_state     (state),
  KEY idx_splitbrain_proofs_opened_at (opened_at)
);

-- ─── 7. proof_approvals （四眼审批）────────────────────────────────────────
CREATE TABLE IF NOT EXISTS proof_approvals (
  id         VARCHAR(64)  NOT NULL PRIMARY KEY,
  proof_id   VARCHAR(64)  NOT NULL,
  approver   VARCHAR(64)  NOT NULL,
  role       VARCHAR(32)  NOT NULL DEFAULT '',
  comment    VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME(3)  NOT NULL,
  KEY idx_proof_approvals_proof_id   (proof_id),
  KEY idx_proof_approvals_approver   (approver)
);

-- ─── 8. restore_gen_record （全局世代水位单例）──────────────────────────────
CREATE TABLE IF NOT EXISTS restore_gen_record (
  id          VARCHAR(16)  NOT NULL PRIMARY KEY DEFAULT 'global',
  current_gen BIGINT       NOT NULL DEFAULT 0,
  updated_at  DATETIME(3)  NOT NULL
);

-- 初始化全局世代水位行（幂等）
INSERT IGNORE INTO restore_gen_record (id, current_gen, updated_at)
VALUES ('global', 0, NOW(3));
