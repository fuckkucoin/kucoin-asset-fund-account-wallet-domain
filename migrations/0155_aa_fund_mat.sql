-- 功能：用户资金物化行、水位、读围栏、视图版本与重建任务。
-- 重点：可用/冻结/在途/占用分栏；重建状态 queued/running/done/failed。
-- 风险点：未追平的投影不得被强一致读当成已覆盖；本表不存放偿付叶。
-- 作者：Axel
CREATE TABLE IF NOT EXISTS fund_mat_rows (
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  subject VARCHAR(32) NOT NULL,
  available VARCHAR(64) NOT NULL,
  frozen VARCHAR(64) NOT NULL,
  in_flight VARCHAR(64) NOT NULL,
  in_flight_deposit VARCHAR(64) NOT NULL,
  in_flight_withdraw VARCHAR(64) NOT NULL,
  in_flight_transfer VARCHAR(64) NOT NULL,
  occupancy VARCHAR(64) NOT NULL,
  occupancy_risk VARCHAR(64) NOT NULL,
  occupancy_reserve VARCHAR(64) NOT NULL,
  causal_id VARCHAR(64) NOT NULL DEFAULT '',
  last_event_id VARCHAR(64) NOT NULL DEFAULT '',
  row_version BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (user_id, asset, subject)
);

CREATE TABLE IF NOT EXISTS fund_mat_events (
  event_id VARCHAR(64) NOT NULL PRIMARY KEY,
  stream VARCHAR(32) NOT NULL,
  seq BIGINT NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  subject VARCHAR(32) NOT NULL,
  op VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  bucket VARCHAR(32) NOT NULL DEFAULT '',
  causal_id VARCHAR(64) NOT NULL DEFAULT '',
  source_event_id VARCHAR(64) NOT NULL DEFAULT '',
  occurred_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_fund_mat_ev_seq (stream, seq),
  KEY idx_fund_mat_ev_user (user_id, stream, seq)
);

CREATE TABLE IF NOT EXISTS fund_mat_applied (
  event_id VARCHAR(64) NOT NULL PRIMARY KEY,
  stream VARCHAR(32) NOT NULL,
  seq BIGINT NOT NULL,
  applied_at DATETIME(3) NOT NULL,
  KEY idx_fund_mat_applied_stream (stream, seq)
);

CREATE TABLE IF NOT EXISTS fund_mat_wm (
  stream VARCHAR(32) NOT NULL PRIMARY KEY,
  epoch BIGINT NOT NULL DEFAULT 1,
  head_seq BIGINT NOT NULL DEFAULT 0,
  contiguous_seq BIGINT NOT NULL DEFAULT 0,
  applied_count BIGINT NOT NULL DEFAULT 0,
  hole_count BIGINT NOT NULL DEFAULT 0,
  lag_events BIGINT NOT NULL DEFAULT 0,
  lag_ms BIGINT NOT NULL DEFAULT 0,
  sla_ms BIGINT NOT NULL DEFAULT 0,
  within_sla TINYINT NOT NULL DEFAULT 1,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS read_fences (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  stream VARCHAR(32) NOT NULL DEFAULT '',
  epoch BIGINT NOT NULL DEFAULT 0,
  min_seq BIGINT NOT NULL DEFAULT 0,
  token VARCHAR(64) NOT NULL DEFAULT '',
  issued_at DATETIME(3) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  KEY idx_read_fences_user (user_id, issued_at)
);

CREATE TABLE IF NOT EXISTS view_versions (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  scope VARCHAR(16) NOT NULL,
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  version BIGINT NOT NULL,
  epoch BIGINT NOT NULL DEFAULT 1,
  wm_seq BIGINT NOT NULL DEFAULT 0,
  digest CHAR(64) NOT NULL,
  row_count INT NOT NULL DEFAULT 0,
  built_at DATETIME(3) NOT NULL,
  KEY idx_view_versions_user (user_id, version)
);

CREATE TABLE IF NOT EXISTS rebuild_jobs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  scope VARCHAR(16) NOT NULL,
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  tolerance VARCHAR(32) NOT NULL DEFAULT '0',
  drift_count INT NOT NULL DEFAULT 0,
  drift_sample VARCHAR(255) NOT NULL DEFAULT '',
  error_text VARCHAR(255) NOT NULL DEFAULT '',
  queued_at DATETIME(3) NOT NULL,
  started_at DATETIME(3) NULL,
  finished_at DATETIME(3) NULL,
  KEY idx_rebuild_jobs_status (status, queued_at)
);

CREATE TABLE IF NOT EXISTS fund_mat_alerts (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  stream VARCHAR(32) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  detail VARCHAR(255) NOT NULL DEFAULT '',
  open_flag TINYINT NOT NULL DEFAULT 1,
  raised_at DATETIME(3) NOT NULL,
  KEY idx_fund_mat_alerts_open (open_flag, raised_at)
);

CREATE TABLE IF NOT EXISTS fund_mat_source (
  source_id VARCHAR(64) NOT NULL PRIMARY KEY,
  seen_at DATETIME(3) NOT NULL
);
