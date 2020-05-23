-- 资金副作用恰好一次引擎 — 数据库表定义。
-- 新增四张表：fund_effects / delivery_gens / dup_proofs / kind_policies。
-- 不改写任何既有 aa_ 表；表前缀 fx_ 与既有 aa_ 隔离。
-- 作者：Kinsley
-- 金额列保留 DECIMAL(36,18) 规范；时间列 DATETIME(3)（精度 1ms）。
--
-- 设计原则：
--   fund_effects    — FundEffect 唯一效果记录（effect_key 数据库唯一约束实现双花封堵）；
--   delivery_gens   — 投递世代 CAS 记录（business_key + expected_gen 联合唯一）；
--   dup_proofs      — 双花证明永久存档（只写不删，供审计和事故复盘）；
--   kind_policies   — Kind 策略配置（运营可热更新）。
--
-- 注意：
--   fx_fund_effects.effect_key 的唯一索引即是双花封堵的数据库层最后防线；
--   应用层 CAS 失败后，数据库唯一约束提供最终保障（双重防护）。
--
-- 内部主机占位（不修改，保持产品契约不变）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16）

-- ────────────────────────────────────────────────────────────────────────────
-- fx_fund_effects: 资金副作用唯一效果记录
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fx_fund_effects (
  id               VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '全局唯一 UUID（应用层生成）',

  business_key     VARCHAR(191)   NOT NULL DEFAULT ''
    COMMENT '业务主键（如 deposit_id、withdrawal_id、transfer_id）',

  kind             VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '资金副作用类型（credit_intent / freeze_intent / hop_intent / notify_fund 等）',

  generation       BIGINT         NOT NULL DEFAULT 1
    COMMENT '投递世代号（从 1 开始，每次重新入箱递增）',

  -- effect_key = hex(SHA256(business_key + ":" + generation))[:32]
  -- 此列的唯一约束是双花封堵的数据库层最终防线；冲突 = 双花。
  effect_key       VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'SHA256(business_key:generation)[:32] hex；唯一约束实现双花封堵',

  state            VARCHAR(16)    NOT NULL DEFAULT 'proposed'
    COMMENT '效果状态：proposed / applied / rejected / poisoned',

  payload          TEXT           NOT NULL DEFAULT ''
    COMMENT '原始业务载荷 JSON 快照（只读存储，不参与 CAS）',

  applied_at       DATETIME(3)    NULL
    COMMENT '效果被 applied 的时刻（NULL = 尚未 applied）',

  rejected_at      DATETIME(3)    NULL
    COMMENT '效果被 rejected 或 poisoned 的时刻',

  reject_reason    VARCHAR(32)    NOT NULL DEFAULT ''
    COMMENT '拒绝原因码：stale_gen / dup_gen / poisoned / kind_blocked / handler_error',

  poison_retries   INT            NOT NULL DEFAULT 0
    COMMENT '触发毒丸时的累计重试次数',

  force_replay_at  DATETIME(3)    NULL
    COMMENT '强制重放时刻（双人授权后解除 poisoned 的时刻）',

  force_replay_by  VARCHAR(191)   NOT NULL DEFAULT ''
    COMMENT '强制重放操作人（格式：actor1+actor2）',

  created_at       DATETIME(3)    NOT NULL
    COMMENT '记录创建时刻',

  updated_at       DATETIME(3)    NOT NULL
    COMMENT '最后状态变更时刻',

  -- 数据库层唯一约束：同 effect_key 不可二次写入（最终双花防线）
  UNIQUE KEY uk_fx_fund_effects_key   (effect_key),

  -- 按 business_key 查询历史效果（desk / 复盘）
  KEY idx_fx_fund_effects_biz        (business_key, generation),

  -- 按 state + created_at 做 Worker 巡检扫描
  KEY idx_fx_fund_effects_state      (state, created_at),

  -- 按 kind + state 做统计分析
  KEY idx_fx_fund_effects_kind_state (kind, state, created_at)

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='资金副作用唯一效果记录；effect_key 唯一约束封堵异步投递双花';
  applied_gen      BIGINT         NOT NULL DEFAULT 0
    COMMENT '最后一次被 applied 的世代号（0 = 从未 applied）',

  state            VARCHAR(16)    NOT NULL DEFAULT 'enqueued'
    COMMENT '世代状态：enqueued / inflight / acked / nacked / poisoned',

  retry_count      INT            NOT NULL DEFAULT 0
    COMMENT '累计失败重试次数（不含首次投递）',

  max_retry        INT            NOT NULL DEFAULT 5
    COMMENT '允许的最大重试次数（超限进毒丸）',

  reject_reason    VARCHAR(32)    NOT NULL DEFAULT ''
    COMMENT '最近一次被拒绝的原因码',

  effect_id        VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '关联的 fx_fund_effects.id',

  outbox_event_id  VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '对应的出箱事件 ID（跨模块审计关联）',

  acked_at         DATETIME(3)    NULL
    COMMENT 'handler 成功 Mark 的时刻',

  nacked_at        DATETIME(3)    NULL
    COMMENT '最近一次 nack 时刻',

  created_at       DATETIME(3)    NOT NULL
    COMMENT '记录创建时刻',

  updated_at       DATETIME(3)    NOT NULL
    COMMENT '最后状态变更时刻',

  -- business_key + expected_gen 联合唯一：防同一世代重复注册
  UNIQUE KEY uk_fx_gens_biz_gen     (business_key, expected_gen),

  -- 关联 effect 查询
  KEY idx_fx_gens_effect            (effect_id),

  -- 巡检扫描（inflight 超时、nacked 待重试）
  KEY idx_fx_gens_state             (state, updated_at),

  -- business_key 所有世代历史查询
  KEY idx_fx_gens_biz               (business_key, expected_gen)

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='投递世代 CAS 记录；expected_gen CAS 防旧世代重投双花';

-- ────────────────────────────────────────────────────────────────────────────
-- fx_dup_proofs: 双花证明永久存档
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fx_dup_proofs (
  id                  VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT '全局唯一 UUID',

  business_key        VARCHAR(191)   NOT NULL DEFAULT ''
    COMMENT '触发双花的业务主键',

  kind                VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '资金副作用类型',

  winner_gen          BIGINT         NOT NULL DEFAULT 0
    COMMENT '胜者投递世代号（CAS 通过的世代）',

  loser_gen           BIGINT         NOT NULL DEFAULT 0
    COMMENT '败者投递世代号（被 CAS 拒绝的世代）',

  winner_effect_id    VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '胜者 fx_fund_effects.id',

  loser_effect_id     VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '败者 fx_fund_effects.id（可为空，若败者从未建 effect）',

  winner_delivery_id  VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '胜者 fx_delivery_gens.id',

  loser_delivery_id   VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '败者 fx_delivery_gens.id',

  winner_at           DATETIME(3)    NOT NULL
    COMMENT '胜者 Mark 时刻',

  loser_at            DATETIME(3)    NOT NULL
    COMMENT '败者被拒绝时刻',

  -- window_ms = loser_at - winner_at（毫秒，可为负表示乱序）
  window_ms           BIGINT         NOT NULL DEFAULT 0
    COMMENT '双花时间窗口（毫秒）；反映分布式时钟偏差上界',

  reject_code         VARCHAR(32)    NOT NULL DEFAULT ''
    COMMENT '拒绝码：stale_gen / dup_gen / poisoned',

  reviewed            TINYINT(1)     NOT NULL DEFAULT 0
    COMMENT '是否已被 desk 人工审阅（0=未审阅，1=已审阅）',

  reviewed_by         VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT 'desk 审阅人',

  reviewed_at         DATETIME(3)    NULL
    COMMENT 'desk 审阅时刻',

  created_at          DATETIME(3)    NOT NULL
    COMMENT '证明创建时刻',

  -- 按 business_key 查询所有双花证明（desk / 复盘）
  KEY idx_fx_dup_proofs_biz          (business_key, created_at),

  -- desk 日常巡查未审阅列表
  KEY idx_fx_dup_proofs_unreviewed   (reviewed, created_at),

  -- 按 kind 统计各资金 kind 的双花频率
  KEY idx_fx_dup_proofs_kind         (kind, created_at)

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='双花证明永久存档；只写不删；供审计和事故复盘';

-- ────────────────────────────────────────────────────────────────────────────
-- fx_kind_policies: Kind 策略配置
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS fx_kind_policies (
  kind             VARCHAR(64)    NOT NULL PRIMARY KEY
    COMMENT 'kind 字符串（如 credit_intent / freeze_intent / notify_fund 等）',

  gate_required    TINYINT(1)     NOT NULL DEFAULT 1
    COMMENT '是否必须通过效果闸（1=强制幂等，0=允许旁路）',

  blocked          TINYINT(1)     NOT NULL DEFAULT 0
    COMMENT '是否被显式封禁（1=拒绝任何投递）',

  max_retry        INT            NOT NULL DEFAULT 5
    COMMENT '此 kind 的最大重试次数（0=使用全局默认）',

  memo             VARCHAR(255)   NOT NULL DEFAULT ''
    COMMENT '配置变更备注（运营审计）',

  updated_by       VARCHAR(64)    NOT NULL DEFAULT ''
    COMMENT '最后修改人',

  updated_at       DATETIME(3)    NOT NULL
    COMMENT '最后修改时刻'

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='Kind 策略配置；运营可热更新；决定 kind 是否强制走效果闸';

-- 预填充默认资金 kind 策略（gate_required=1；blocked=0）
INSERT IGNORE INTO fx_kind_policies (kind, gate_required, blocked, max_retry, memo, updated_by, updated_at)
VALUES
  ('credit_intent',  1, 0, 5, 'default fund kind — gate required', 'system', NOW(3)),
  ('freeze_intent',  1, 0, 5, 'default fund kind — gate required', 'system', NOW(3)),
  ('hop_intent',     1, 0, 5, 'default fund kind — gate required', 'system', NOW(3)),
  ('notify_fund',    1, 0, 5, 'default fund kind — gate required', 'system', NOW(3)),
  ('debit_intent',   1, 0, 5, 'default fund kind — gate required', 'system', NOW(3)),
  ('settle_intent',  1, 0, 5, 'default fund kind — gate required', 'system', NOW(3));
