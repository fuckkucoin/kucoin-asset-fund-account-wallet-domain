-- 功能：错贷记发现与追回冲销编排引擎持久化表。
-- 边界：这些表不写账本分录；账本分录由 wrongcredit.Orchestrator 通过 LedgerApplier
--       提交至 ledger_entries 表（已有表，本迁移不修改）。
-- 作者：Kinsley
--       wrong_credit_case 只存案件元数据和 FSM 状态；证据内容存 wrong_credit_evidence；
--       时间轴只追加不删改。
-- 不变量：
--   wrong_credit_case.id 唯一；idempotency_key 全局唯一。
--   同一 credit_ref 至多一行 phase NOT IN ('reversed','partial','waived','bad_debt')。
--   wrong_credit_evidence.case_id FK 到 wrong_credit_case.id（软约束，应用层保证）。
--   wrong_credit_mutex.credit_ref 唯一；kind IN ('reorg','wrong_credit')。
--   wrong_credit_timeline.seq 在同一 case_id 内单调递增（应用层保证）。

-- ─── 主案件表 ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wrong_credit_case (
  id                VARCHAR(64)    NOT NULL PRIMARY KEY,
  idempotency_key   VARCHAR(128)   NOT NULL UNIQUE,
  user_id           VARCHAR(128)   NOT NULL,

  -- 原始贷记单据信息
  credit_ref        VARCHAR(128)   NOT NULL,
  asset             VARCHAR(32)    NOT NULL,
  credited_amt      DECIMAL(36,18) NOT NULL,

  -- 发现评分
  confidence        VARCHAR(16)    NOT NULL DEFAULT 'unscored',
  risk_tier         VARCHAR(8)     NOT NULL DEFAULT 'tier1',
  score_note        VARCHAR(512)   NOT NULL DEFAULT '',
  score_version     INT            NOT NULL DEFAULT 0,

  -- 追回结果
  clawed_amt        DECIMAL(36,18) NULL,
  remain_amt        DECIMAL(36,18) NULL,
  reversal_ref      VARCHAR(128)   NULL,

  -- 生命周期
  phase             VARCHAR(32)    NOT NULL DEFAULT 'investigating',
  step              VARCHAR(32)    NOT NULL DEFAULT 'open',
  fence_epoch       BIGINT         NOT NULL DEFAULT 1,
  fence_token       VARCHAR(64)    NOT NULL DEFAULT '',

  -- 审批
  approved_by       VARCHAR(128)   NULL,
  approved_at       DATETIME(3)    NULL,
  approved_by_2     VARCHAR(128)   NULL,
  approved_at_2     DATETIME(3)    NULL,

  -- 豁免
  waived_by         VARCHAR(128)   NULL,
  waived_at         DATETIME(3)    NULL,
  waived_note       VARCHAR(512)   NULL,

  -- 操作人
  opened_by         VARCHAR(128)   NOT NULL DEFAULT '',
  closed_by         VARCHAR(128)   NULL,
  note              VARCHAR(512)   NOT NULL DEFAULT '',

  created_at        DATETIME(3)    NOT NULL,
  updated_at        DATETIME(3)    NOT NULL,

  -- 索引：按用户/资产/状态快速查询
  KEY idx_wcc_user_id       (user_id, created_at),
  KEY idx_wcc_credit_ref    (credit_ref),
  KEY idx_wcc_phase         (phase, created_at),
  KEY idx_wcc_asset_phase   (asset, phase),
  KEY idx_wcc_updated       (updated_at)
);

-- ─── 证据信封表 ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wrong_credit_evidence (
  id         VARCHAR(64)  NOT NULL PRIMARY KEY,
  case_id    VARCHAR(64)  NOT NULL,
  source     VARCHAR(32)  NOT NULL,   -- desk_report / recon_hint / post_hoc_attribution
  sha256     VARCHAR(128) NOT NULL DEFAULT '',
  kind       VARCHAR(64)  NOT NULL,
  note       VARCHAR(512) NOT NULL DEFAULT '',
  -- payload 仅 desk_report 小 blob 内联；其余来源须为空
  payload    TEXT         NULL,
  actor      VARCHAR(128) NOT NULL,
  score      DOUBLE       NOT NULL DEFAULT 0,
  created_at DATETIME(3)  NOT NULL,

  KEY idx_wce_case_id   (case_id, created_at),
  KEY idx_wce_source    (source, created_at)
);

-- ─── 时间轴表 ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS wrong_credit_timeline (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  case_id    VARCHAR(64)  NOT NULL,
  seq        INT          NOT NULL,
  kind       VARCHAR(64)  NOT NULL,
  actor      VARCHAR(128) NOT NULL,
  from_phase VARCHAR(32)  NOT NULL DEFAULT '',
  to_phase   VARCHAR(32)  NOT NULL DEFAULT '',
  from_step  VARCHAR(32)  NOT NULL DEFAULT '',
  to_step    VARCHAR(32)  NOT NULL DEFAULT '',
  note       VARCHAR(512) NOT NULL DEFAULT '',
  at         DATETIME(3)  NOT NULL,

  UNIQUE KEY uk_wct_case_seq (case_id, seq),
  KEY idx_wct_case_at (case_id, at)
);

-- ─── 互斥锁表 ────────────────────────────────────────────────────────────────
-- 节点重启后从此表重建内存 CreditMutex 状态；运行时以内存为准。

CREATE TABLE IF NOT EXISTS wrong_credit_mutex (
  credit_ref VARCHAR(128) NOT NULL PRIMARY KEY,
  kind       VARCHAR(16)  NOT NULL,   -- 'reorg' | 'wrong_credit'
  owner      VARCHAR(128) NOT NULL,   -- batchID 或 caseID
  claimed_at DATETIME(3)  NOT NULL,

  KEY idx_wcm_kind (kind)
);

-- ─── 坏账台账表 ──────────────────────────────────────────────────────────────
-- 记录每笔挂坏账的明细；聚合统计和定期核销用。

CREATE TABLE IF NOT EXISTS wrong_credit_baddebt (
  id           VARCHAR(64)    NOT NULL PRIMARY KEY,
  case_id      VARCHAR(64)    NOT NULL,
  credit_ref   VARCHAR(128)   NOT NULL,
  user_id      VARCHAR(128)   NOT NULL,
  asset        VARCHAR(32)    NOT NULL,
  credited_amt DECIMAL(36,18) NOT NULL,  -- 原始贷记金额
  clawed_amt   DECIMAL(36,18) NOT NULL,  -- 已追回金额
  remain_amt   DECIMAL(36,18) NOT NULL,  -- 挂坏账金额
  reversal_ref VARCHAR(128)   NULL,      -- 冲销分录 TransferID
  phase        VARCHAR(32)    NOT NULL,  -- partial | bad_debt
  created_at   DATETIME(3)    NOT NULL,

  KEY idx_wcbd_case_id    (case_id),
  KEY idx_wcbd_user_id    (user_id, created_at),
  KEY idx_wcbd_asset      (asset, created_at),
  KEY idx_wcbd_credit_ref (credit_ref)
);
