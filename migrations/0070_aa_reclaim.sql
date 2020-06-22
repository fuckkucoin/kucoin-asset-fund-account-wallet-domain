-- 功能：错链/未认领充值回收与迟贷记竞态编排引擎持久化表。
-- 边界：
--   - 这些表不写账本分录；账本操作通过 reclaim.LedgerApplier 接口注入。
-- 作者：Kinsley
--   - reclaim_case 只存案件元数据和 FSM 状态。
--   - reclaim_occupancy 存链上输出预占记录；支持进程重启后恢复内存状态。
--   - reclaim_timeline 只追加不删改。
--   - reclaim_sweep_leg 为案件 reclaimed 后的退款/归集腿目录。
--   - reclaim_race_event 存迟贷记竞态事件，运营消费后可标记 consumed。
-- 不变量：
--   - reclaim_case.id 全局唯一；idempotency_key 全局唯一。
--   - 同一 (tx_hash, vout) 至多一行 state NOT IN ('reclaimed','cancelled','auto_credited')。
--   - reclaim_occupancy.output_key = tx_hash + ':' + vout；UNIQUE。
--   - reclaim_timeline.seq 在同一 case_id 内单调递增（应用层保证）。
--   - reclaim_sweep_leg.case_id 对应 state='reclaimed' 的 reclaim_case.id。

-- ─── 主案件表 ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reclaim_case (
  id                VARCHAR(64)    NOT NULL PRIMARY KEY,
  idempotency_key   VARCHAR(128)   NOT NULL UNIQUE,

  -- 链上输出
  tx_hash           VARCHAR(128)   NOT NULL,
  vout              INT            NOT NULL DEFAULT 0,
  chain             VARCHAR(32)    NOT NULL,
  asset             VARCHAR(32)    NOT NULL,
  amount            DECIMAL(36,18) NOT NULL,

  -- 回收类型
  kind              VARCHAR(32)    NOT NULL,  -- wrong_chain / unclaimed / memo_mismatch / expired_address

  -- 关联用户（unclaimed 场景可为空）
  user_id           VARCHAR(128)   NOT NULL DEFAULT '',

  -- 目标退款地址
  refund_address    VARCHAR(256)   NOT NULL DEFAULT '',

  -- 运营备注
  note              VARCHAR(512)   NOT NULL DEFAULT '',
  opened_by         VARCHAR(128)   NOT NULL DEFAULT '',

  -- 预占信息
  reserved_by       VARCHAR(128)   NULL,
  reserved_at       DATETIME(3)    NULL,

  -- 回收结果
  reclaimed_tx_hash VARCHAR(128)   NULL,
  reclaimed_by      VARCHAR(128)   NULL,
  reclaimed_at      DATETIME(3)    NULL,

  -- 取消信息
  cancelled_by      VARCHAR(128)   NULL,
  cancelled_at      DATETIME(3)    NULL,
  cancel_reason     VARCHAR(512)   NULL,

  -- 升级信息
  escalated_by      VARCHAR(128)   NULL,
  escalated_at      DATETIME(3)    NULL,
  escalate_note     VARCHAR(512)   NULL,

  -- 自动贷记信息
  auto_credit_ref   VARCHAR(128)   NULL,
  auto_credit_at    DATETIME(3)    NULL,

  -- SLA 截止时间
  sla_open_deadline    DATETIME(3) NOT NULL,
  sla_reserve_deadline DATETIME(3) NULL,

  -- FSM 状态
  state             VARCHAR(32)    NOT NULL DEFAULT 'open',
  fence_epoch       BIGINT         NOT NULL DEFAULT 1,
  fence_token       VARCHAR(64)    NOT NULL DEFAULT '',

  created_at        DATETIME(3)    NOT NULL,
  updated_at        DATETIME(3)    NOT NULL,

  -- 唯一约束：同一输出至多一个活跃案件（终态可多行）
  KEY idx_rc_output       (tx_hash, vout, state),
  KEY idx_rc_user_id      (user_id, created_at),
  KEY idx_rc_chain_asset  (chain, asset, state),
  KEY idx_rc_state        (state, created_at),
  KEY idx_rc_updated      (updated_at)
);

-- ─── 链上输出预占表 ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reclaim_occupancy (
  output_key        VARCHAR(192)   NOT NULL PRIMARY KEY,  -- tx_hash:vout
  case_id           VARCHAR(64)    NOT NULL,
  chain             VARCHAR(32)    NOT NULL,
  asset             VARCHAR(32)    NOT NULL,
  amount            DECIMAL(36,18) NOT NULL,
  occupied_at       DATETIME(3)    NOT NULL,

  UNIQUE KEY uk_rco_case_id   (case_id),
  KEY idx_rco_chain_asset     (chain, asset)
);

-- ─── 时间轴表 ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reclaim_timeline (
  id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  case_id      VARCHAR(64)  NOT NULL,
  seq          INT          NOT NULL,
  kind         VARCHAR(64)  NOT NULL,
  actor        VARCHAR(128) NOT NULL,
  from_state   VARCHAR(32)  NOT NULL DEFAULT '',
  to_state     VARCHAR(32)  NOT NULL,
  note         VARCHAR(512) NOT NULL DEFAULT '',
  at           DATETIME(3)  NOT NULL,

  UNIQUE KEY uk_rct_case_seq  (case_id, seq),
  KEY idx_rct_case_at         (case_id, at)
);

-- ─── Sweep leg 表 ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reclaim_sweep_leg (
  id           VARCHAR(64)   NOT NULL PRIMARY KEY,
  case_id      VARCHAR(64)   NOT NULL,
  output_key   VARCHAR(192)  NOT NULL,
  chain        VARCHAR(32)   NOT NULL,
  asset        VARCHAR(32)   NOT NULL,
  amount       DECIMAL(36,18) NOT NULL,
  seq          INT            NOT NULL,
  kind         VARCHAR(64)    NOT NULL,  -- occupancy_release / refund_broadcast / refund_confirmed / sweep_to_hot / proof_anchor
  tx_hash      VARCHAR(128)   NULL,
  proof_hash   VARCHAR(128)   NULL,
  note         VARCHAR(512)   NOT NULL DEFAULT '',
  actor        VARCHAR(128)   NOT NULL,
  at           DATETIME(3)    NOT NULL,

  UNIQUE KEY uk_rsl_case_seq  (case_id, seq),
  KEY idx_rsl_case_id         (case_id),
  KEY idx_rsl_chain_asset     (chain, asset, at)
);

-- ─── 迟贷记竞态事件表 ────────────────────────────────────────────────────────
-- 存储 LateCreditMutex 生成的竞态事件，运营消费后标记 consumed=1。

CREATE TABLE IF NOT EXISTS reclaim_race_event (
  id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  output_key   VARCHAR(192) NOT NULL,
  case_id      VARCHAR(64)  NOT NULL,
  chain        VARCHAR(32)  NOT NULL,
  asset        VARCHAR(32)  NOT NULL,
  amount       DECIMAL(36,18) NOT NULL,
  transfer_id  VARCHAR(128) NOT NULL,
  reason       VARCHAR(128) NOT NULL,
  consumed     TINYINT      NOT NULL DEFAULT 0,
  at           DATETIME(3)  NOT NULL,

  KEY idx_rre_case_id    (case_id),
  KEY idx_rre_output_key (output_key, at),
  KEY idx_rre_consumed   (consumed, at)
);
