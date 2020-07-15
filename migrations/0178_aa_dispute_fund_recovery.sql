-- 争议退单资金追偿与四眼收口编排。
-- 建立 dispute_fund_plans、recovery_legs、close_gates、close_proofs 表。
-- 不改广播跟踪表，不改错贷记追回表，不改出金失败补偿表。
-- 作者：Axel
-- 本迁移由争议/退单 Case 驱动，与运营错贷记发现、出金失败补偿完全隔离。
;

-- 争议资金计划表：每个 Case 至多一条，由结案前必须完成的资金腿集合组成。
CREATE TABLE IF NOT EXISTS dispute_fund_plans (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  case_id VARCHAR(64) NOT NULL COMMENT '关联 disputedesk.Case.ID（只读引用）',
  channel VARCHAR(16) NOT NULL COMMENT 'fiat|p2p',
  user_id VARCHAR(64) NOT NULL COMMENT '争议发起用户',
  asset VARCHAR(32) NOT NULL COMMENT '争议资产',
  amount VARCHAR(64) NOT NULL COMMENT '争议金额（DECIMAL 字符串）',
  plan_type VARCHAR(32) NOT NULL COMMENT 'claw_back|refund|restore',
  status VARCHAR(32) NOT NULL COMMENT 'open|executing|closed|aborted',
  rail_break_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '法币轨道断裂 ID（只读挂 E 包）',
  opened_by VARCHAR(64) NOT NULL COMMENT '开单人（开 Case 的操作人）',
  closed_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '收口操作人',
  abort_reason VARCHAR(255) NOT NULL DEFAULT '' COMMENT '中止原因',
  version INT NOT NULL DEFAULT 1 COMMENT '乐观锁版本号',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_dispute_fund_plans_case (case_id),
  KEY idx_dispute_fund_plans_status (status),
  KEY idx_dispute_fund_plans_user (user_id, asset),
  KEY idx_dispute_fund_plans_created (created_at)
) COMMENT='争议资金计划（每 Case 一条；plan_type 决定追偿/退还/恢复方向）';

-- 追偿腿表：一个 FundPlan 下有多条 RecoveryLeg，每条对应一个账务动作。
CREATE TABLE IF NOT EXISTS recovery_legs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL COMMENT '关联 dispute_fund_plans.id',
  case_id VARCHAR(64) NOT NULL COMMENT '冗余存 case_id，便于查询',
  seq INT NOT NULL COMMENT '腿序号（从 1 起）',
  leg_type VARCHAR(32) NOT NULL COMMENT 'freeze|claw_debit|refund_credit|restore_unfreeze|skip_marker',
  user_id VARCHAR(64) NOT NULL COMMENT '受影响用户',
  asset VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL COMMENT '腿金额（正数 DECIMAL 字符串）',
  status VARCHAR(32) NOT NULL COMMENT 'pending|posting|posted|failed|skipped',
  idempotency_key VARCHAR(128) NOT NULL COMMENT '腿幂等键（plan_id+seq）',
  ledger_ref VARCHAR(64) NOT NULL DEFAULT '' COMMENT '账本分录 ID（只读引用）',
  hold_ref VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'holdcat hold ID（只读引用）',
  rail_break_ref VARCHAR(64) NOT NULL DEFAULT '' COMMENT '法币轨道断裂 ID（只读挂钩）',
  leg_hash VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'SHA256(plan_id+seq+type+amount+status)',
  posted_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '执行人（四眼不得与开单人同人）',
  note VARCHAR(255) NOT NULL DEFAULT '',
  posted_at DATETIME(3) NULL COMMENT '腿完成时间',
  failed_at DATETIME(3) NULL COMMENT '腿失败时间',
  fail_reason VARCHAR(255) NOT NULL DEFAULT '',
  retry_count INT NOT NULL DEFAULT 0,
  next_retry_at DATETIME(3) NULL COMMENT '下次重试时间',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_recovery_legs_idem (idempotency_key),
  KEY idx_recovery_legs_plan (plan_id, seq),
  KEY idx_recovery_legs_case (case_id),
  KEY idx_recovery_legs_status (status, next_retry_at),
  KEY idx_recovery_legs_user (user_id, asset)
) COMMENT='追偿腿（freeze/claw/refund/restore；幂等；状态机 pending→posted/failed/skipped）';

-- 结案收口闸表：绑定 Plan 与 Case 结案，确保所有腿终态后才放行结案。
CREATE TABLE IF NOT EXISTS close_gates (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL COMMENT '关联 dispute_fund_plans.id',
  case_id VARCHAR(64) NOT NULL COMMENT '关联 disputedesk.Case.ID',
  gate_status VARCHAR(32) NOT NULL COMMENT 'blocking|passed|bypassed',
  blocking_reason VARCHAR(255) NOT NULL DEFAULT '' COMMENT '阻断原因（pending 腿列表摘要）',
  pending_leg_ids TEXT NOT NULL DEFAULT '' COMMENT '未完成腿 ID 列表（逗号分隔）',
  checked_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '收口审核人',
  passed_at DATETIME(3) NULL COMMENT '放行时间',
  bypass_reason VARCHAR(255) NOT NULL DEFAULT '' COMMENT '豁免原因（特批）',
  bypassed_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '豁免操作人',
  version INT NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_close_gates_plan (plan_id),
  UNIQUE KEY uk_close_gates_case (case_id),
  KEY idx_close_gates_status (gate_status),
  KEY idx_close_gates_created (created_at)
) COMMENT='结案收口闸（blocking=阻断 Resolve；passed=允许结案；bypassed=管理员特批）';

-- 收口证明表：Case 结案时生成，含所有腿哈希集合，用于完整性巡检。
CREATE TABLE IF NOT EXISTS close_proofs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL COMMENT '关联 dispute_fund_plans.id',
  case_id VARCHAR(64) NOT NULL COMMENT '关联 disputedesk.Case.ID',
  gate_id VARCHAR(64) NOT NULL COMMENT '关联 close_gates.id',
  proof_hash VARCHAR(64) NOT NULL COMMENT 'SHA256(所有腿哈希有序拼接) 防篡改根哈希',
  leg_hashes TEXT NOT NULL COMMENT '各腿 leg_hash 有序 JSON 数组',
  leg_count INT NOT NULL COMMENT '腿总数',
  total_amount VARCHAR(64) NOT NULL COMMENT '各腿金额之和',
  asset VARCHAR(32) NOT NULL,
  decided_by VARCHAR(64) NOT NULL COMMENT '结案决策人',
  decision VARCHAR(32) NOT NULL COMMENT 'resolve|reject',
  tamper_detected TINYINT(1) NOT NULL DEFAULT 0 COMMENT '巡检发现篡改标记',
  tamper_detail VARCHAR(255) NOT NULL DEFAULT '' COMMENT '篡改详情',
  tamper_checked_at DATETIME(3) NULL COMMENT '最近一次巡检时间',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_close_proofs_plan (plan_id),
  UNIQUE KEY uk_close_proofs_case (case_id),
  KEY idx_close_proofs_tamper (tamper_detected),
  KEY idx_close_proofs_created (created_at)
) COMMENT='收口证明（结案时快照所有腿哈希；篡改检测由 CloseGate 巡检 worker 执行）';
