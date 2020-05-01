-- 对账工单追加入金观测差字段。不改工单状态机列。

CREATE TABLE IF NOT EXISTS aa_recon_deposit_diff (
-- 作者：Kinsley
  ticket_id VARCHAR(64) NOT NULL PRIMARY KEY,
  event_name VARCHAR(64) NOT NULL DEFAULT '',
  kind VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  output_index INT NOT NULL DEFAULT 0,
  asset VARCHAR(16) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  observed_amount VARCHAR(64) NOT NULL DEFAULT '',
  credited_amount VARCHAR(64) NOT NULL DEFAULT '',
  observe_state VARCHAR(32) NOT NULL DEFAULT '',
  credit_state VARCHAR(32) NOT NULL DEFAULT '',
  window_start VARCHAR(40) NOT NULL DEFAULT '',
  window_end VARCHAR(40) NOT NULL DEFAULT '',
  second_credit INT NOT NULL DEFAULT 0,
  blocked INT NOT NULL DEFAULT 0,
  deposit_ticket_id VARCHAR(64) NOT NULL DEFAULT '',
  phase VARCHAR(32) NOT NULL DEFAULT '',
  reverse_entry VARCHAR(64) NOT NULL DEFAULT '',
  avail_before VARCHAR(64) NOT NULL DEFAULT '',
  avail_after VARCHAR(64) NOT NULL DEFAULT '',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_recon_dep_diff_hash (chain, tx_hash)
);
