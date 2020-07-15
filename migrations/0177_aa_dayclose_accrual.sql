-- 日终清算轧差异常挂账与跨日应计引擎。
-- 建立 day_closes、accrual_batches、accrual_lines、suspense_entries、except_links 表。
-- 不改广播跟踪表，不改 recon 分类器表，不改三方对账公式表。
-- 作者：TianpengLan
;

-- 日终封锁记录表
CREATE TABLE IF NOT EXISTS day_closes (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  business_day DATE NOT NULL COMMENT '营业日（日历意义）',
  status VARCHAR(32) NOT NULL COMMENT 'open|freezing|closed|reopened_limited',
  freeze_at DATETIME(3) NULL COMMENT '封锁窗开始时间',
  closed_at DATETIME(3) NULL COMMENT '日切完成时间',
  reopened_at DATETIME(3) NULL COMMENT '有限再开时间',
  opened_by VARCHAR(64) NOT NULL COMMENT '开日操作人',
  frozen_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '封锁操作人',
  closed_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关日操作人',
  reopened_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '再开操作人',
  reopen_reason VARCHAR(255) NOT NULL DEFAULT '' COMMENT '再开原因',
  is_holiday TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否为假日跳过',
  catch_up_day DATE NULL COMMENT '假日对应的补跑工作日',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_day_closes_biz_day (business_day, status),
  KEY idx_day_closes_status (status),
  KEY idx_day_closes_created (created_at)
) COMMENT='日终封锁记录';

-- 跨日应计批次表
CREATE TABLE IF NOT EXISTS accrual_batches (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  business_day DATE NOT NULL COMMENT '产生应计的营业日',
  day_close_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联 day_close.id',
  orig_batch_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '冲正时引用原批次 ID',
  status VARCHAR(32) NOT NULL COMMENT 'draft|posted|reversed',
  version INT NOT NULL DEFAULT 1 COMMENT '版本号，单调递增',
  total_debit VARCHAR(64) NOT NULL DEFAULT '0' COMMENT '借方合计（DECIMAL 字符串）',
  total_credit VARCHAR(64) NOT NULL DEFAULT '0' COMMENT '贷方合计（DECIMAL 字符串）',
  actor VARCHAR(64) NOT NULL COMMENT '操作人',
  note VARCHAR(255) NOT NULL DEFAULT '',
  posted_at DATETIME(3) NULL,
  reversed_at DATETIME(3) NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_accrual_batches_day (business_day),
  KEY idx_accrual_batches_status (status),
  KEY idx_accrual_batches_dayclose (day_close_id)
) COMMENT='跨日应计批次';

-- 应计分录表
CREATE TABLE IF NOT EXISTS accrual_lines (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  batch_id VARCHAR(64) NOT NULL COMMENT '所属批次',
  seq INT NOT NULL COMMENT '批次内分录序号（从 1 递增）',
  kind VARCHAR(64) NOT NULL COMMENT 'accrued_interest|accrued_fee|netting_diff|reversal',
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL COMMENT '正数金额（DECIMAL 字符串）',
  direction VARCHAR(8) NOT NULL COMMENT 'debit|credit',
  counter_ref VARCHAR(64) NOT NULL DEFAULT '' COMMENT '对应分录 ID（借贷互引）',
  movement_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '在途 movement 引用',
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_accrual_lines_batch_seq (batch_id, seq),
  KEY idx_accrual_lines_batch (batch_id),
  KEY idx_accrual_lines_user (user_id, asset)
) COMMENT='应计分录（借贷对称）';

-- 挂账科目分录表
CREATE TABLE IF NOT EXISTS suspense_entries (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  batch_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '来源清算批次',
  item_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '来源清算 Item',
  exception_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联 clearing.Exception.ID',
  recon_diff_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'recon 差异单只读引用',
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL COMMENT '挂账金额（正数，DECIMAL 字符串）',
  direction VARCHAR(8) NOT NULL COMMENT 'debit|credit',
  reason VARCHAR(255) NOT NULL DEFAULT '' COMMENT '挂账原因',
  status VARCHAR(32) NOT NULL COMMENT 'open|resolved|rejected',
  note VARCHAR(255) NOT NULL DEFAULT '' COMMENT '处置备注',
  escalate_at DATETIME(3) NOT NULL COMMENT 'SLA 升级触发时间',
  escalated_at DATETIME(3) NULL COMMENT '已升级时间',
  resolved_at DATETIME(3) NULL,
  rejected_at DATETIME(3) NULL,
  actor VARCHAR(64) NOT NULL COMMENT '建立挂账的操作人',
  disposed_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '处置操作人',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_suspense_entries_batch (batch_id),
  KEY idx_suspense_entries_status (status),
  KEY idx_suspense_entries_user (user_id, asset),
  KEY idx_suspense_entries_escalate (escalate_at, status)
) COMMENT='异常挂账科目分录';

-- 清算异常与 recon 差异单关联表
CREATE TABLE IF NOT EXISTS except_links (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  exception_id VARCHAR(64) NOT NULL COMMENT 'clearing.Exception.ID',
  recon_diff_id VARCHAR(64) NOT NULL COMMENT 'recon 差异单号（只读引用）',
  note VARCHAR(255) NOT NULL DEFAULT '' COMMENT '关联说明',
  linked_by VARCHAR(64) NOT NULL,
  linked_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_except_links_pair (exception_id, recon_diff_id),
  KEY idx_except_links_exception (exception_id),
  KEY idx_except_links_diff (recon_diff_id)
) COMMENT='清算异常与对账差异单关联（只读引用，不调分类器）';
