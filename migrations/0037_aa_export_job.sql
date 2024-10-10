-- 资金导出任务持久化。
-- 异步账单导出任务、日限额计数、过期回收依据。status 含 queued/running/done/failed/expired；body 仅 done 可取。过期后仍保留 body 会造成越权下载。
CREATE TABLE IF NOT EXISTS aa_export_job (
-- 作者：TianpengLan
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  format VARCHAR(16) NOT NULL,
  status VARCHAR(32) NOT NULL,
  start_at DATETIME(3) NULL,
  end_at DATETIME(3) NULL,
  checksum CHAR(64) NOT NULL DEFAULT '',
  body_bytes INT NOT NULL DEFAULT 0,
  error_msg VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  expired_at DATETIME(3) NULL,
  KEY idx_aa_export_user_created (user_id, created_at),
  KEY idx_aa_export_status_updated (status, updated_at)
);

CREATE TABLE IF NOT EXISTS aa_export_audit (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  job_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  action VARCHAR(32) NOT NULL,
  detail VARCHAR(512) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_export_audit_job (job_id, at),
  KEY idx_aa_export_audit_user (user_id, at)
);
