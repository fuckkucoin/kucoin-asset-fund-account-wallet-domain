-- 功能：大额出金分片计划、分片腿、部分成交分录、分片冻结占用、拆分策略表。
-- 重点：shard_plans.state 只允许 draft/approved_exec/running/filled/partial_closed/aborted；
--       shard_legs.state 只允许 pending/frozen/broadcasting/filled/failed；
-- 作者：Kinsley
--       Σ shard_legs.amount = shard_plans.total_amount（应用层守恒，DB 层不再约束）；
--       广播后 hold 不得被本包自动释放（live-hash 守恒）。
-- 风险点：不要用触发器更新 amount 字段，守恒由应用层保证并在事务内落分录。
CREATE TABLE IF NOT EXISTS shard_plans (
  id                VARCHAR(64)  NOT NULL PRIMARY KEY,
  user_id           VARCHAR(64)  NOT NULL,
  asset             VARCHAR(16)  NOT NULL,
  chain             VARCHAR(32)  NOT NULL,
  to_address        VARCHAR(255) NOT NULL,
  total_amount      VARCHAR(64)  NOT NULL,
  policy_id         VARCHAR(64)  NOT NULL,
  batch_id          VARCHAR(64)  NOT NULL DEFAULT '',
  withdraw_id       VARCHAR(128) NOT NULL DEFAULT '',
  state             VARCHAR(24)  NOT NULL,
  leg_count         INT          NOT NULL DEFAULT 0,
  filled_amount     VARCHAR(64)  NOT NULL DEFAULT '0',
  inflight_amount   VARCHAR(64)  NOT NULL DEFAULT '0',
  remain_amount     VARCHAR(64)  NOT NULL DEFAULT '0',
  actor             VARCHAR(64)  NOT NULL DEFAULT '',
  pause_reason      VARCHAR(255) NOT NULL DEFAULT '',
  abort_reason      VARCHAR(255) NOT NULL DEFAULT '',
  idempotency_key   VARCHAR(191) NOT NULL DEFAULT '',
  created_at        DATETIME(3)  NOT NULL,
  updated_at        DATETIME(3)  NOT NULL,
  KEY idx_shard_plans_user    (user_id, state),
  KEY idx_shard_plans_batch   (batch_id),
  KEY idx_shard_plans_withdraw(withdraw_id),
  UNIQUE KEY uk_shard_plans_idem (idempotency_key)
);

CREATE TABLE IF NOT EXISTS shard_legs (
  id             VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id        VARCHAR(64)  NOT NULL,
  seq            INT          NOT NULL,
  amount         VARCHAR(64)  NOT NULL,
  state          VARCHAR(20)  NOT NULL,
  hold_id        VARCHAR(64)  NOT NULL DEFAULT '',
  fill_id        VARCHAR(64)  NOT NULL DEFAULT '',
  withdraw_id    VARCHAR(128) NOT NULL DEFAULT '',
  broadcast_ref  VARCHAR(128) NOT NULL DEFAULT '',
  fail_reason    VARCHAR(255) NOT NULL DEFAULT '',
  frozen_at      DATETIME(3)  NULL,
  broadcast_at   DATETIME(3)  NULL,
  filled_at      DATETIME(3)  NULL,
  failed_at      DATETIME(3)  NULL,
  created_at     DATETIME(3)  NOT NULL,
  updated_at     DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_shard_leg_seq  (plan_id, seq),
  KEY idx_shard_legs_plan      (plan_id, state),
  KEY idx_shard_legs_hold      (hold_id),
  KEY idx_shard_legs_withdraw  (withdraw_id)
);

CREATE TABLE IF NOT EXISTS shard_holds (
  id             VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id        VARCHAR(64)  NOT NULL,
  leg_id         VARCHAR(64)  NOT NULL,
  user_id        VARCHAR(64)  NOT NULL,
  asset          VARCHAR(16)  NOT NULL,
  amount         VARCHAR(64)  NOT NULL,
  state          VARCHAR(16)  NOT NULL,
  fill_id        VARCHAR(64)  NOT NULL DEFAULT '',
  release_reason VARCHAR(255) NOT NULL DEFAULT '',
  consumed_at    DATETIME(3)  NULL,
  released_at    DATETIME(3)  NULL,
  expires_at     DATETIME(3)  NULL,
  created_at     DATETIME(3)  NOT NULL,
  updated_at     DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_shard_holds_leg  (leg_id),
  KEY idx_shard_holds_plan       (plan_id, state),
  KEY idx_shard_holds_user_asset (user_id, asset, state)
);

CREATE TABLE IF NOT EXISTS partial_fills (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id      VARCHAR(64)  NOT NULL,
  leg_id       VARCHAR(64)  NOT NULL,
  hold_id      VARCHAR(64)  NOT NULL,
  user_id      VARCHAR(64)  NOT NULL,
  asset        VARCHAR(16)  NOT NULL,
  amount       VARCHAR(64)  NOT NULL,
  fee_amount   VARCHAR(64)  NOT NULL DEFAULT '0',
  tx_hash      VARCHAR(128) NOT NULL DEFAULT '',
  chain        VARCHAR(32)  NOT NULL,
  state        VARCHAR(16)  NOT NULL,
  confirmed_at DATETIME(3)  NULL,
  created_at   DATETIME(3)  NOT NULL,
  updated_at   DATETIME(3)  NOT NULL,
  KEY idx_partial_fills_plan    (plan_id, state),
  KEY idx_partial_fills_leg     (leg_id),
  KEY idx_partial_fills_hold    (hold_id),
  KEY idx_partial_fills_user    (user_id, asset, state),
  KEY idx_partial_fills_tx      (tx_hash)
);

CREATE TABLE IF NOT EXISTS plan_policies (
  id               VARCHAR(64)  NOT NULL PRIMARY KEY,
  name             VARCHAR(128) NOT NULL,
  max_legs         INT          NOT NULL DEFAULT 8,
  min_leg_amount   VARCHAR(64)  NOT NULL,
  max_leg_amount   VARCHAR(64)  NOT NULL,
  hot_cap_fraction VARCHAR(32)  NOT NULL DEFAULT '0.8',
  split_mode       VARCHAR(16)  NOT NULL DEFAULT 'equal',
  allow_partial    TINYINT(1)   NOT NULL DEFAULT 1,
  description      VARCHAR(255) NOT NULL DEFAULT '',
  created_at       DATETIME(3)  NOT NULL,
  KEY idx_plan_policies_name (name)
);
