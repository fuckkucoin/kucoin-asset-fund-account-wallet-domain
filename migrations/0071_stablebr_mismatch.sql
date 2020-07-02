-- 功能：稳定币桥 mint/burn 证明失配检测持久化表。
-- 边界：
--   - brmismatch_case 只存案件元数据和 FSM 状态；不写账本分录。
-- 作者：TianpengLan
--   - brmismatch_proof 持久化双边守恒证明哈希，append-only。
--   - brmismatch_suspense 记录账户暂停状态及活跃案件计数。
--   - brmismatch_suspense_case 关联暂停账户与导致暂停的案件（多对多）。
-- 不变量：
--   - brmismatch_case.id 全局唯一（UUIDv4）。
--   - 同一 (chain_event_hash, entry_hash) 且 state NOT IN ('cleared') 至多一行。
--   - brmismatch_proof.case_id 对应 brmismatch_case.id；UNIQUE。
--   - brmismatch_suspense.suspended_key = user_id + '|' + asset；UNIQUE。
--   - brmismatch_suspense.count >= 0；count=0 时不阻断操作。
--   - state 仅允许单向前进：open → suspended → cleared。

-- ─── 失配案件主表 ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS brmismatch_case (
  id                VARCHAR(64)     NOT NULL PRIMARY KEY,
  kind              VARCHAR(32)     NOT NULL,   -- mint_over / burn_over / mint_miss / burn_miss
  state             VARCHAR(32)     NOT NULL DEFAULT 'open',

  user_id           VARCHAR(128)    NOT NULL DEFAULT '',
  asset             VARCHAR(32)     NOT NULL,   -- USDT / USDC
  chain_amount      DECIMAL(36,18)  NULL,
  book_amount       DECIMAL(36,18)  NULL,

  chain_event_hash  VARCHAR(64)     NOT NULL,
  entry_hash        VARCHAR(64)     NOT NULL,
  proof_hash        VARCHAR(64)     NOT NULL,
  window_key        VARCHAR(64)     NOT NULL,

  suspended_by      VARCHAR(128)    NOT NULL DEFAULT '',
  suspended_at      DATETIME(3)     NULL,

  cleared_by_1      VARCHAR(128)    NOT NULL DEFAULT '',
  cleared_by_2      VARCHAR(128)    NOT NULL DEFAULT '',
  cleared_at        DATETIME(3)     NULL,

  note              VARCHAR(512)    NOT NULL DEFAULT '',

  detected_at       DATETIME(3)     NOT NULL,
  updated_at        DATETIME(3)     NOT NULL,

  -- 同一事件对不得重复开立活跃案件
  KEY idx_bmc_event_pair  (chain_event_hash, entry_hash, state),
  KEY idx_bmc_user_asset  (user_id, asset, state),
  KEY idx_bmc_asset_state (asset, state, detected_at),
  KEY idx_bmc_updated     (updated_at)
);

-- ─── 双边守恒证明表（append-only）────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS brmismatch_proof (
  case_id           VARCHAR(64)     NOT NULL PRIMARY KEY,
  chain_event_hash  VARCHAR(64)     NOT NULL,
  entry_hash        VARCHAR(64)     NOT NULL,
  window_key        VARCHAR(64)     NOT NULL,
  proof_hash        VARCHAR(64)     NOT NULL,
  persisted_at      DATETIME(3)     NOT NULL,

  KEY idx_bmp_proof_hash (proof_hash),
  KEY idx_bmp_window     (window_key, persisted_at)
);

-- ─── 账户暂停状态表 ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS brmismatch_suspense (
  suspended_key     VARCHAR(192)    NOT NULL PRIMARY KEY,  -- user_id|asset
  user_id           VARCHAR(128)    NOT NULL,
  asset             VARCHAR(32)     NOT NULL,
  active_count      INT             NOT NULL DEFAULT 0,    -- 活跃暂停案件数
  suspended_at      DATETIME(3)     NOT NULL,
  last_updated_at   DATETIME(3)     NOT NULL,

  KEY idx_bms_user_id (user_id),
  KEY idx_bms_asset   (asset, active_count)
);

-- ─── 暂停账户↔案件关联表 ─────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS brmismatch_suspense_case (
  id                BIGINT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
  suspended_key     VARCHAR(192)    NOT NULL,
  case_id           VARCHAR(64)     NOT NULL,
  linked_at         DATETIME(3)     NOT NULL,

  UNIQUE KEY uk_bmsc_key_case (suspended_key, case_id),
  KEY idx_bmsc_case_id        (case_id)
);

-- ─── 检测窗口日志表 ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS brmismatch_window_log (
  id                BIGINT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
  window_key        VARCHAR(64)     NOT NULL,
  asset             VARCHAR(32)     NOT NULL,
  chain_event_count INT             NOT NULL DEFAULT 0,
  book_entry_count  INT             NOT NULL DEFAULT 0,
  matched_count     INT             NOT NULL DEFAULT 0,
  mismatch_count    INT             NOT NULL DEFAULT 0,
  ran_at            DATETIME(3)     NOT NULL,

  KEY idx_bmwl_window_asset (window_key, asset),
  KEY idx_bmwl_ran_at       (ran_at)
);
