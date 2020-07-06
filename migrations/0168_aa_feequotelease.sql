-- 手续费报价租约漂移封堵引擎：数据库表定义。
--
-- 设计原则：
--   1. fee_quote_leases：每次 QuoteLease 创建一行，绑定 sch_gen + tiers 指纹；
--      ApplyBound 复验后更新状态至 applied；Schedule 变更后更新至 revoked；
--      TTL 超时后 Worker 更新至 expired。
--   2. fee_sch_generations：每次 PutSchedule 抬升 gen，记录档位版本历史；
--      每个 kind/asset 维护最高 gen（upsert 语义）。
--   3. fee_quote_proofs：漂移事故证明，只追加，不修改；
--      供下游对账与运营审计只读消费。
--
-- 与前轮隔离：
--   - 不共享 vipfee / feecredit / feeoracle 表（三包已占核心 FSM）；
--   - 不修改 aa_fee_schedules 表（feeeng 内核，只读消费）；
--   - fee_quote_lease_id / sch_gen 仅在本三表中写入，供他包只读引用。
--
-- 作者：CoreFeeLeaseEngine
-- 日期：2020.12.04

-- ──────────────────────────────────────────────────────────────────────────────
-- 1. 报价租约表
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fee_quote_leases (
  id                VARCHAR(64)   NOT NULL                  COMMENT '租约唯一标识（UUID）',
  kind              VARCHAR(32)   NOT NULL                  COMMENT '手续费种类（withdraw/deposit/transfer/hop/earn_redeem）',
  asset             VARCHAR(16)   NOT NULL                  COMMENT '资产代码（如 BTC/USDT/*）',
  gross             VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '报价本金金额',
  volume            VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '30 日名义量（分档用，可为空）',
  quoted_bps        INT           NOT NULL DEFAULT 0        COMMENT '报价 BPS',
  quoted_fee        VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '报价费用',
  quoted_net        VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '报价净额',
  schedule_id       VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '绑定 Schedule.ID',
  sch_gen           BIGINT        NOT NULL DEFAULT 0        COMMENT '绑定的 Schedule 世代序号',
  tiers_fingerprint VARCHAR(128)  NOT NULL DEFAULT ''       COMMENT 'tiers SHA-256 指纹（空串=未分档）',
  state             VARCHAR(16)   NOT NULL DEFAULT 'quoted' COMMENT 'FSM 状态：quoted/applied/expired/revoked',
  expires_at        DATETIME(3)   NOT NULL                  COMMENT 'TTL 到期时刻',
  quoted_at         DATETIME(3)   NOT NULL                  COMMENT '签发时刻',
  applied_at        DATETIME(3)            DEFAULT NULL     COMMENT 'Apply 消费时刻',
  expired_at        DATETIME(3)            DEFAULT NULL     COMMENT '过期时刻',
  revoked_at        DATETIME(3)            DEFAULT NULL     COMMENT '吊销时刻',
  revoke_reason     VARCHAR(512)  NOT NULL DEFAULT ''       COMMENT '吊销原因',
  idempotency_key   VARCHAR(128)  NOT NULL DEFAULT ''       COMMENT '幂等键',
  updated_at        DATETIME(3)   NOT NULL                  COMMENT '最近更新时刻',
  PRIMARY KEY (id),
  UNIQUE  KEY uk_fee_quote_lease_idem   (idempotency_key),
  KEY     idx_fee_quote_lease_state     (state, expires_at),
  KEY     idx_fee_quote_lease_kind_asset(kind, asset, state, quoted_at),
  KEY     idx_fee_quote_lease_sch_gen   (sch_gen, state),
  KEY     idx_fee_quote_lease_updated   (updated_at)
) COMMENT='手续费报价租约（fee_quote_lease_id 供他包只读消费）';

-- ──────────────────────────────────────────────────────────────────────────────
-- 2. 档位表世代记录（每 kind/asset 一行，upsert 更新）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fee_sch_generations (
  id                VARCHAR(64)   NOT NULL                  COMMENT '世代记录唯一标识（UUID）',
  kind              VARCHAR(32)   NOT NULL                  COMMENT '手续费种类',
  asset             VARCHAR(16)   NOT NULL                  COMMENT '资产代码',
  gen               BIGINT        NOT NULL DEFAULT 1        COMMENT '单调递增世代序号（sch_gen）',
  schedule_id       VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '触发本次世代的 Schedule.ID',
  bps               INT           NOT NULL DEFAULT 0        COMMENT '本次生效 BPS',
  tiers_fingerprint VARCHAR(128)  NOT NULL DEFAULT ''       COMMENT '本次生效 tiers 指纹',
  put_at            DATETIME(3)   NOT NULL                  COMMENT 'PutSchedule 发生时刻',
  put_by            VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '操作来源（ops user / system）',
  PRIMARY KEY (id),
  UNIQUE  KEY uk_fee_sch_gen_kind_asset (kind, asset),
  KEY     idx_fee_sch_gen_put_at        (put_at),
  KEY     idx_fee_sch_gen_gen           (gen)
) COMMENT='档位表世代历史（sch_gen 供他包只读消费）';

-- ──────────────────────────────────────────────────────────────────────────────
-- 3. 漂移事故证明仓（只追加，不修改）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fee_quote_proofs (
  id                  VARCHAR(64)   NOT NULL                  COMMENT '证明唯一标识（UUID）',
  kind                VARCHAR(32)   NOT NULL                  COMMENT '证明类型（sch_gen_advanced/fingerprint_mismatch/lease_expired/...）',
  lease_id            VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '对应 FeeQuoteLease.ID',
  fee_kind            VARCHAR(32)   NOT NULL DEFAULT ''       COMMENT '手续费种类',
  asset               VARCHAR(16)   NOT NULL DEFAULT ''       COMMENT '资产代码',
  gross               VARCHAR(64)   NOT NULL DEFAULT ''       COMMENT '原始本金',
  bind_sch_gen        BIGINT        NOT NULL DEFAULT 0        COMMENT '租约绑定时的 sch_gen',
  current_sch_gen     BIGINT        NOT NULL DEFAULT 0        COMMENT 'Apply 时当前 sch_gen',
  bind_fingerprint    VARCHAR(128)  NOT NULL DEFAULT ''       COMMENT '绑定时 tiers 指纹',
  current_fingerprint VARCHAR(128)  NOT NULL DEFAULT ''       COMMENT '当前 tiers 指纹',
  bind_bps            INT           NOT NULL DEFAULT 0        COMMENT '绑定时 BPS',
  current_bps         INT           NOT NULL DEFAULT 0        COMMENT '当前 BPS',
  evidence            VARCHAR(4096) NOT NULL DEFAULT ''       COMMENT '事故现场摘要（JSON）',
  detected_at         DATETIME(3)   NOT NULL                  COMMENT '检测时刻',
  exported_at         DATETIME(3)            DEFAULT NULL     COMMENT '已导出时刻（空=未导出）',
  PRIMARY KEY (id),
  KEY idx_fee_quote_proofs_lease      (lease_id, detected_at),
  KEY idx_fee_quote_proofs_kind       (kind, detected_at),
  KEY idx_fee_quote_proofs_fee_kind   (fee_kind, asset, detected_at),
  KEY idx_fee_quote_proofs_export     (exported_at, detected_at),
  KEY idx_fee_quote_proofs_detected   (detected_at)
) COMMENT='手续费报价档位漂移事故证明仓（只追加）';
