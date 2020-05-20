-- 并发可用校验与超提竞态屏障引擎数据库表。
-- 表前缀与既有 aa_ 体系保持一致。
-- 金额列 DECIMAL(36,18)；时间列 DATETIME(3)（精度 1ms）。
-- 作者：Kinsley
-- 不改写任何既有 aa_ 表或列；新增表前缀：avail_ / debit_ / race_。
--
-- 设计说明：
--   avail_leases    — 可用快照租约，存续 ≤ 30s 短窗口，
--                      Gen 单调递增，绑定后不可二次绑定；
--   debit_intents   — 扣减/冻结意图，携带 lease_gen + expected_avail 参与 CAS；
--   race_proofs     — 竞态证明，记录同窗口 CAS 冲突的 winner/loser 对；
--   barrier_metrics — 屏障运行期聚合计数（每日快照，desk 用）。
--
-- ────────────────────────────────────────────────────────────────────────────
-- avail_leases: 可用快照租约主表
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS avail_leases (
  id           VARCHAR(64)     NOT NULL PRIMARY KEY  COMMENT '全局唯一 UUID',
  user_id      VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT '用户标识',
  asset        VARCHAR(16)     NOT NULL DEFAULT ''   COMMENT '资产代码（如 USDT、BTC）',
  book         VARCHAR(32)     NOT NULL DEFAULT ''   COMMENT '账本位（如 main、earn）',
  -- gen 在同 user_id×asset×book 内单调递增；意图 CAS 检验使用。
  gen          BIGINT          NOT NULL DEFAULT 0    COMMENT '世代号，单调递增',
  -- 快照时刻余额分解（只读汇入，不写任何外部科目）。
  balance      DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT '账本总余额快照',
  frozen       DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT 'holdcat 已冻额快照',
  reserved     DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT '热钱包预留金快照（只读）',
  in_flight    DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT '当前窗口已接受意图扣减总额',
  available    DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT '净可用余额 = balance-frozen-reserved-in_flight',
  -- 租约状态：open → bound → expired/revoked
  state        VARCHAR(16)     NOT NULL DEFAULT 'open' COMMENT '状态：open/bound/expired/revoked',
  expires_at   DATETIME(3)     NOT NULL               COMMENT '租约到期时刻（默认 +30s）',
  created_at   DATETIME(3)     NOT NULL               COMMENT '租约颁发时刻',
  updated_at   DATETIME(3)     NOT NULL               COMMENT '最后状态变更时刻',

  KEY idx_avail_leases_user_asset   (user_id, asset, book, state),
  KEY idx_avail_leases_state_exp    (state, expires_at),
  KEY idx_avail_leases_gen          (user_id, asset, book, gen)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='可用快照租约；短窗口 CAS 互斥证明基础';

-- ────────────────────────────────────────────────────────────────────────────
-- debit_intents: 扣减/冻结意图主表
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS debit_intents (
  id             VARCHAR(64)     NOT NULL PRIMARY KEY  COMMENT '全局唯一 UUID',
  idem_key       VARCHAR(128)    NOT NULL DEFAULT ''   COMMENT '幂等键（出金 ID 或划转 ID 派生）',
  user_id        VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT '用户标识',
  asset          VARCHAR(16)     NOT NULL DEFAULT ''   COMMENT '资产代码',
  book           VARCHAR(32)     NOT NULL DEFAULT ''   COMMENT '账本位',
  -- 意图金额，必须 > 0。
  amount         DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT '意图扣减/冻结额',
  kind           VARCHAR(16)     NOT NULL DEFAULT ''   COMMENT '操作类型：debit/freeze',
  reason         VARCHAR(128)    NOT NULL DEFAULT ''   COMMENT '业务语义说明',
  -- CAS 携带字段：必须与对应租约的 gen 和 available 完全一致。
  lease_id       VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT '关联租约 ID',
  lease_gen      BIGINT          NOT NULL DEFAULT 0    COMMENT '携带的租约世代号',
  expected_avail DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT '携带的期望可用余额',
  -- 意图状态：proposed → accepted/rejected
  state          VARCHAR(16)     NOT NULL DEFAULT 'proposed' COMMENT '状态：proposed/accepted/rejected',
  reject_code    VARCHAR(32)     NOT NULL DEFAULT ''   COMMENT '拒绝原因码（rejected 时填充）',
  reject_note    VARCHAR(512)    NOT NULL DEFAULT ''   COMMENT '拒绝说明（人工可读）',
  race_proof_id  VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT '竞态证明 ID（被拒绝且有证明时填充）',
  -- 审计字段。
  actor_id       VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT '发起方服务标识',
  request_ip     VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT '来源 IP（审计）',
  proposed_at    DATETIME(3)     NOT NULL               COMMENT '意图提交时刻',
  decided_at     DATETIME(3)     NULL                   COMMENT '屏障裁决时刻',

  UNIQUE KEY uq_debit_intents_idem     (idem_key),
  KEY        idx_debit_intents_lease   (lease_id, state),
  KEY        idx_debit_intents_user    (user_id, asset, book, state),
  KEY        idx_debit_intents_proof   (race_proof_id),
  KEY        idx_debit_intents_time    (proposed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='扣减/冻结意图；携带 CAS 字段，屏障裁决后落终态';

-- ────────────────────────────────────────────────────────────────────────────
-- race_proofs: 竞态证明主表
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS race_proofs (
  id               VARCHAR(64)     NOT NULL PRIMARY KEY  COMMENT '全局唯一 UUID',
  user_id          VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT '竞态发生的用户',
  asset            VARCHAR(16)     NOT NULL DEFAULT ''   COMMENT '竞态资产代码',
  book             VARCHAR(32)     NOT NULL DEFAULT ''   COMMENT '竞态账本位',
  lease_id         VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT '共享的租约 ID',
  -- winner/loser 意图引用。
  winner_intent_id VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT 'CAS 胜者意图 ID（可为空，超提场景无胜者）',
  loser_intent_id  VARCHAR(64)     NOT NULL DEFAULT ''   COMMENT 'CAS 败者意图 ID',
  winner_amount    DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT '胜者意图金额',
  loser_amount     DECIMAL(36,18)  NOT NULL DEFAULT 0    COMMENT '败者意图金额',
  -- CAS 裁决时刻的可用余额快照。
  avail_at_decision DECIMAL(36,18) NOT NULL DEFAULT 0   COMMENT '裁决时净可用余额',
  -- 竞争窗口（租约 created_at ~ 拒绝裁决时刻）。
  window_start     DATETIME(3)     NOT NULL               COMMENT '竞争窗口起始（租约 created_at）',
  window_end       DATETIME(3)     NOT NULL               COMMENT '竞争窗口结束（拒绝时刻）',
  reject_code      VARCHAR(32)     NOT NULL DEFAULT ''   COMMENT '败者拒绝原因码',
  note             VARCHAR(512)    NOT NULL DEFAULT ''   COMMENT '补充说明',
  created_at       DATETIME(3)     NOT NULL               COMMENT '证明落库时刻',

  KEY idx_race_proofs_user     (user_id, asset, created_at),
  KEY idx_race_proofs_lease    (lease_id),
  KEY idx_race_proofs_loser    (loser_intent_id),
  KEY idx_race_proofs_winner   (winner_intent_id),
  KEY idx_race_proofs_code     (reject_code, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='竞态证明；记录同窗口 CAS 冲突 winner/loser 对，供 desk 与事故复盘';

-- ────────────────────────────────────────────────────────────────────────────
-- barrier_metrics: 屏障运行期聚合计数（每日快照）
-- ────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS barrier_metrics (
  id                   VARCHAR(64)  NOT NULL PRIMARY KEY  COMMENT '全局唯一 UUID',
  snapshot_date        DATE         NOT NULL               COMMENT '快照日期（UTC 日期）',
  -- 租约计数。
  leases_issued        BIGINT       NOT NULL DEFAULT 0     COMMENT '颁发租约总数',
  leases_expired       BIGINT       NOT NULL DEFAULT 0     COMMENT '自然过期租约数',
  leases_revoked       BIGINT       NOT NULL DEFAULT 0     COMMENT '被撤销租约数',
  -- 意图计数。
  intents_accepted     BIGINT       NOT NULL DEFAULT 0     COMMENT '已接受意图数',
  intents_rejected     BIGINT       NOT NULL DEFAULT 0     COMMENT '已拒绝意图数',
  -- 超提与竞态。
  overdraw_attempts    BIGINT       NOT NULL DEFAULT 0     COMMENT '超提企图次数',
  race_proofs_created  BIGINT       NOT NULL DEFAULT 0     COMMENT '竞态证明创建数',
  -- 时间戳。
  created_at           DATETIME(3)  NOT NULL               COMMENT '快照写入时刻',
  updated_at           DATETIME(3)  NOT NULL               COMMENT '最后更新时刻',

  UNIQUE KEY uq_barrier_metrics_date (snapshot_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='屏障引擎每日聚合指标快照，供 desk 运营面板展示';
