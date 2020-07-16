-- P2P 卖单保证金超时弃单与释放竞态收口——表结构迁移。
-- 建立 p2p_abandon_plans、fill_release_tokens、hold_ages、p2p_close_proofs 四张表。
-- 不改 holdcat 分层 taxonomy 表，不改 dispute 追偿表，不改出金失败补偿表。
-- 作者：Axel
-- 本迁移由 P2P 卖单订单时钟（order_id + timer）驱动，与争议 Case 追偿完全隔离。
;

-- ─────────────────────────── p2p_abandon_plans 弃单计划表 ───────────────────────────
-- 每个 P2P 卖单超时后生成唯一一条弃单计划（order_id 唯一约束保证）。
-- 状态机：armed → abandoning → released | filled | conflict。
CREATE TABLE IF NOT EXISTS p2p_abandon_plans (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  order_id VARCHAR(64) NOT NULL COMMENT 'P2P 卖单 ID（驱动源；唯一约束）',
  user_id VARCHAR(64) NOT NULL COMMENT '保证金所有人',
  asset VARCHAR(32) NOT NULL COMMENT '保证金资产',
  amount VARCHAR(64) NOT NULL COMMENT '保证金金额（DECIMAL 字符串）',
  hold_id VARCHAR(64) NOT NULL COMMENT 'holdcat hold 行 ID（只读引用）',
  status VARCHAR(32) NOT NULL COMMENT 'armed|abandoning|released|filled|conflict',
  armed_at DATETIME(3) NOT NULL COMMENT '计划就绪时间',
  abandoning_at DATETIME(3) NULL COMMENT '进入 abandoning 时间',
  terminal_at DATETIME(3) NULL COMMENT '终态时间',
  terminal_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '终态执行人（system 或 operator_id）',
  conflict_note VARCHAR(512) NOT NULL DEFAULT '' COMMENT 'conflict 状态时的冲突说明',
  timeout_secs BIGINT NOT NULL DEFAULT 1800 COMMENT '保证金超时阈值（秒）',
  grace_secs BIGINT NOT NULL DEFAULT 60 COMMENT '宽限期（秒）',
  sla_escalate_secs BIGINT NOT NULL DEFAULT 86400 COMMENT 'SLA 升级阈值（秒）',
  version INT NOT NULL DEFAULT 1 COMMENT '乐观锁版本号',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_p2p_abandon_plans_order (order_id),
  KEY idx_p2p_abandon_plans_status (status),
  KEY idx_p2p_abandon_plans_user (user_id, asset),
  KEY idx_p2p_abandon_plans_armed (armed_at),
  KEY idx_p2p_abandon_plans_created (created_at)
) COMMENT='P2P 卖单弃单计划（每卖单一条；status 决定弃单/成交/冲突资金路径）';

-- ─────────────────────────── fill_release_tokens 互斥令牌表 ───────────────────────────
-- 保证同一 plan_id 下的 release 与 fill 路径互斥：先 committed 的令牌锁定终态。
-- kind: release=弃单释放路径，fill=迟成交贷记路径。
-- status: reserved → committed | aborted。
CREATE TABLE IF NOT EXISTS fill_release_tokens (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL COMMENT '关联 p2p_abandon_plans.id',
  order_id VARCHAR(64) NOT NULL COMMENT '冗余 P2P 卖单 ID（日志/排查用）',
  kind VARCHAR(16) NOT NULL COMMENT 'release|fill',
  status VARCHAR(16) NOT NULL COMMENT 'reserved|committed|aborted',
  reserved_by VARCHAR(64) NOT NULL COMMENT '预留操作人',
  committed_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '提交操作人',
  aborted_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '中止操作人',
  abort_reason VARCHAR(255) NOT NULL DEFAULT '' COMMENT '中止原因',
  conflict_detected TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否检测到竞态冲突',
  ledger_ref VARCHAR(64) NOT NULL DEFAULT '' COMMENT '账本分录 ID（只读）',
  hold_ref VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'holdcat hold ID（只读）',
  reserved_at DATETIME(3) NOT NULL,
  terminal_at DATETIME(3) NULL,
  version INT NOT NULL DEFAULT 1 COMMENT '乐观锁版本号',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_fill_release_tokens_plan (plan_id),
  KEY idx_fill_release_tokens_kind_status (kind, status),
  KEY idx_fill_release_tokens_order (order_id)
) COMMENT='P2P 弃单互斥令牌（release vs fill 路径互斥；先 committed 者锁定终态）';

-- ─────────────────────────── hold_ages 保证金超龄记录表 ───────────────────────────
-- 记录 P2P 卖单保证金超龄情况，按 SLA 阈值分级（normal/warning/escalated/resolved）。
-- 超龄超过 SLA 时自动升级至 desk 干预；禁止无限悬挂。
CREATE TABLE IF NOT EXISTS hold_ages (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL COMMENT '关联 p2p_abandon_plans.id',
  order_id VARCHAR(64) NOT NULL COMMENT 'P2P 卖单 ID',
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  hold_id VARCHAR(64) NOT NULL COMMENT 'holdcat hold 行 ID（只读）',
  stage VARCHAR(16) NOT NULL COMMENT 'normal|warning|escalated|resolved',
  hold_age_secs BIGINT NOT NULL DEFAULT 0 COMMENT '当前 hold 已持续秒数（巡检时快照）',
  sla_secs BIGINT NOT NULL DEFAULT 86400 COMMENT 'SLA 阈值（秒）',
  escalated_at DATETIME(3) NULL COMMENT '升级时间',
  escalated_to VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'desk 操作人/组',
  escalate_note VARCHAR(512) NOT NULL DEFAULT '' COMMENT '升级说明',
  resolved_at DATETIME(3) NULL COMMENT '处置时间',
  resolved_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '处置人',
  resolve_note VARCHAR(512) NOT NULL DEFAULT '' COMMENT '处置说明',
  patrol_count INT NOT NULL DEFAULT 0 COMMENT '已巡检次数',
  last_patrol DATETIME(3) NULL COMMENT '最后巡检时间',
  version INT NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_hold_ages_plan (plan_id),
  KEY idx_hold_ages_stage (stage),
  KEY idx_hold_ages_order (order_id),
  KEY idx_hold_ages_user (user_id, asset),
  KEY idx_hold_ages_escalated (escalated_at)
) COMMENT='P2P 保证金超龄记录（按 SLA 分级；超龄升级至 desk 干预）';

-- ─────────────────────────── p2p_close_proofs 收口证明表 ───────────────────────────
-- 弃单/释放/成交三选一终态证明（CloseProof）。
-- proof_hash = SHA256(plan_id|order_id|outcome|committed_token_id|amount|asset)。
-- 缺失 CloseProof 时资金面不得标 closed（validate_close API 强制检查）。
CREATE TABLE IF NOT EXISTS p2p_close_proofs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL COMMENT '关联 p2p_abandon_plans.id',
  order_id VARCHAR(64) NOT NULL COMMENT 'P2P 卖单 ID',
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  outcome VARCHAR(16) NOT NULL COMMENT 'released|filled|conflict',
  committed_token_id VARCHAR(64) NOT NULL COMMENT '已提交互斥令牌 ID',
  committed_kind VARCHAR(16) NOT NULL COMMENT 'release|fill',
  proof_hash VARCHAR(64) NOT NULL COMMENT 'SHA256 防篡改哈希',
  hold_ref VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'holdcat hold ID（只读）',
  ledger_ref VARCHAR(64) NOT NULL DEFAULT '' COMMENT '账本分录 ID（只读）',
  decided_by VARCHAR(128) NOT NULL COMMENT '生成者（system 或 desk 双人标识）',
  tamper_detected TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否检测到篡改',
  tamper_detail VARCHAR(512) NOT NULL DEFAULT '' COMMENT '篡改详情',
  tamper_checked_at DATETIME(3) NULL COMMENT '最后篡改检查时间',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_p2p_close_proofs_plan (plan_id),
  KEY idx_p2p_close_proofs_order (order_id),
  KEY idx_p2p_close_proofs_outcome (outcome),
  KEY idx_p2p_close_proofs_created (created_at)
) COMMENT='P2P 弃单收口证明（三选一终态；缺失时禁止标 closed）';
