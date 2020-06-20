-- 功能：双边失配暂停清障世代与出金挡写互斥收口引擎——数据库表结构。
-- 重点：
--   suspense_gen_counters 记录 (user_id, asset) 维度的世代计数器；
--     current_gen 为已签发的最大世代号，active_count 为当前活跃暂停数。
--     counter_key 由应用层构造（suspgen:counter:<userID>:<asset>），唯一主键。
--   suspense_gens 记录每个暂停世代的详细状态，含 FSM 字段和四眼清障信息；
--     state 枚举 active/clearing/cleared；cleared_at 仅在 cleared 后填写。
--     gen_id 由应用层 sha256 构造，全局唯一；user_id+asset+gen_value 联合唯一。
--   clearance_leases 由 FourEyesClear 成功后签发，绑定到 (user_id, asset, gen_value)；
--     state 枚举 issued/consumed/expired；expire_at 为 UTC 短 TTL（默认 5 分钟）。
--     同一 (user_id, asset, gen_value) 仅允许一个 issued 状态 lease（应用层保证）。
--   suspense_proofs 只增审计表，记录挡写、双清、漏拦三类异常事件；
--     禁止 UPDATE；禁止 DELETE。
-- 风险点：
--   gen_id、lease_id、proof_id 均为 VARCHAR(64)，存 sha256-hex 或 UUID；禁止缩短。
--   时间列用 DATETIME(3) UTC 存储，出库后须 .UTC() 转换。
--   suspense_proofs 须用 INSERT IGNORE / ON CONFLICT DO NOTHING；禁止 UPDATE。
--   active_count 与 current_gen 须在同一行 CAS，防止跨列竞态。
-- Author: TianpengLan
-- 作者：Kinsley
-- Date: 2020-11-08

CREATE TABLE IF NOT EXISTS suspense_gen_counters (
  counter_key   VARCHAR(191)  NOT NULL,
  user_id       VARCHAR(64)   NOT NULL,
  asset         VARCHAR(16)   NOT NULL,
  current_gen   BIGINT        NOT NULL DEFAULT 0,
  active_count  INT           NOT NULL DEFAULT 0,
  updated_at    DATETIME(3)   NOT NULL,
  PRIMARY KEY (counter_key),
  KEY idx_sgc_user_asset (user_id, asset)
);

CREATE TABLE IF NOT EXISTS suspense_gens (
  gen_id        VARCHAR(64)   NOT NULL,
  user_id       VARCHAR(64)   NOT NULL,
  asset         VARCHAR(16)   NOT NULL,
  gen_value     BIGINT        NOT NULL DEFAULT 0,
  case_id       VARCHAR(64)   NOT NULL DEFAULT '',
  suspended_by  VARCHAR(64)   NOT NULL DEFAULT '',
  state         VARCHAR(16)   NOT NULL DEFAULT 'active',
  clear_actor1  VARCHAR(64)   NOT NULL DEFAULT '',
  clear_actor2  VARCHAR(64)   NOT NULL DEFAULT '',
  clear_note    VARCHAR(255)  NOT NULL DEFAULT '',
  suspended_at  DATETIME(3)   NOT NULL,
  cleared_at    DATETIME(3)   NULL,
  updated_at    DATETIME(3)   NOT NULL,
  PRIMARY KEY (gen_id),
  UNIQUE KEY uk_sg_user_asset_gen (user_id, asset, gen_value),
  KEY idx_sg_user_asset_state (user_id, asset, state),
  KEY idx_sg_state_suspended (state, suspended_at),
  KEY idx_sg_case_id (case_id)
);

CREATE TABLE IF NOT EXISTS clearance_leases (
  lease_id      VARCHAR(64)   NOT NULL,
  user_id       VARCHAR(64)   NOT NULL,
  asset         VARCHAR(16)   NOT NULL,
  gen_value     BIGINT        NOT NULL DEFAULT 0,
  case_id       VARCHAR(64)   NOT NULL DEFAULT '',
  state         VARCHAR(16)   NOT NULL DEFAULT 'issued',
  issued_at     DATETIME(3)   NOT NULL,
  expire_at     DATETIME(3)   NOT NULL,
  consumed_at   DATETIME(3)   NULL,
  consumed_by   VARCHAR(64)   NOT NULL DEFAULT '',
  PRIMARY KEY (lease_id),
  KEY idx_cl_user_asset_gen (user_id, asset, gen_value),
  KEY idx_cl_state_expire (state, expire_at),
  KEY idx_cl_case_id (case_id)
);

CREATE TABLE IF NOT EXISTS suspense_proofs (
  proof_id      VARCHAR(64)   NOT NULL,
  case_id       VARCHAR(64)   NOT NULL DEFAULT '',
  user_id       VARCHAR(64)   NOT NULL,
  asset         VARCHAR(16)   NOT NULL,
  susp_gen      BIGINT        NOT NULL DEFAULT 0,
  count_before  INT           NOT NULL DEFAULT 0,
  count_after   INT           NOT NULL DEFAULT 0,
  block_outcome VARCHAR(32)   NOT NULL,
  detail        VARCHAR(512)  NOT NULL DEFAULT '',
  proof_hash    VARCHAR(64)   NOT NULL DEFAULT '',
  attempted_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (proof_id),
  KEY idx_sp_case_id (case_id),
  KEY idx_sp_outcome_at (block_outcome, attempted_at),
  KEY idx_sp_user_asset_at (user_id, asset, attempted_at)
);
