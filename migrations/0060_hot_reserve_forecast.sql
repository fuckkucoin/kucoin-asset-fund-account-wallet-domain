-- 功能：热钱包预测快照、预留单、预留分录与三段对齐差异。
-- 重点：预留状态 open/earmarked/consumed/expired/cancelled；金额用十进制字符串。
-- 风险点：对齐差异未关时不得新开预留或签名；已广播预留不得因过期被释放。
-- 作者：TianpengLan
CREATE TABLE IF NOT EXISTS hot_forecast_snapshot (
  asset VARCHAR(16) NOT NULL PRIMARY KEY,
  version BIGINT NOT NULL,
  window_sec BIGINT NOT NULL,
  as_of DATETIME(3) NOT NULL,
  hot_avail VARCHAR(64) NOT NULL,
  hot_frozen VARCHAR(64) NOT NULL,
  point_est VARCHAR(64) NOT NULL,
  lower_bound VARCHAR(64) NOT NULL,
  inflow_exp VARCHAR(64) NOT NULL,
  outflow_exp VARCHAR(64) NOT NULL,
  inflow_kept VARCHAR(64) NOT NULL,
  outflow_stressed VARCHAR(64) NOT NULL,
  sweep_hold VARCHAR(64) NOT NULL,
  in_transit VARCHAR(64) NOT NULL,
  buffer_amt VARCHAR(64) NOT NULL,
  haircut VARCHAR(32) NOT NULL,
  stress VARCHAR(32) NOT NULL,
  inputs_hash CHAR(64) NOT NULL,
  sweep_ids VARCHAR(1024) NOT NULL DEFAULT '',
  sample_n INT NOT NULL DEFAULT 0,
  bias VARCHAR(64) NOT NULL DEFAULT '',
  realized VARCHAR(64) NOT NULL DEFAULT '',
  event_id VARCHAR(128) NOT NULL
);

CREATE TABLE IF NOT EXISTS hot_forecast_history (
  asset VARCHAR(16) NOT NULL,
  version BIGINT NOT NULL,
  inputs_hash CHAR(64) NOT NULL,
  lower_bound VARCHAR(64) NOT NULL,
  point_est VARCHAR(64) NOT NULL,
  as_of DATETIME(3) NOT NULL,
  event_id VARCHAR(128) NOT NULL,
  PRIMARY KEY (asset, version)
);

CREATE TABLE IF NOT EXISTS hot_forecast_sample (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  asset VARCHAR(16) NOT NULL,
  at DATETIME(3) NOT NULL,
  inflow VARCHAR(64) NOT NULL,
  outflow VARCHAR(64) NOT NULL,
  KEY idx_hot_forecast_sample (asset, at)
);

CREATE TABLE IF NOT EXISTS hot_reserve_order (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  idem_key VARCHAR(128) NOT NULL DEFAULT '',
  fingerprint VARCHAR(255) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  state VARCHAR(16) NOT NULL,
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  intent_id VARCHAR(128) NOT NULL DEFAULT '',
  movement_id VARCHAR(128) NOT NULL DEFAULT '',
  approval_ticket_id VARCHAR(128) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  expires_at DATETIME(3) NOT NULL,
  earmarked_at DATETIME(3) NULL,
  consumed_at DATETIME(3) NULL,
  released_at DATETIME(3) NULL,
  release_kind VARCHAR(32) NOT NULL DEFAULT '',
  requester VARCHAR(64) NOT NULL DEFAULT '',
  approver VARCHAR(64) NOT NULL DEFAULT '',
  broadcast_ref VARCHAR(128) NOT NULL DEFAULT '',
  freeze_ref VARCHAR(128) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_hot_reserve_asset_state (asset, state),
  KEY idx_hot_reserve_intent (intent_id),
  KEY idx_hot_reserve_move (movement_id),
  KEY idx_hot_reserve_idem (idem_key)
);

CREATE TABLE IF NOT EXISTS hot_reserve_journal (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  reservation_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  seq INT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  debit_subject VARCHAR(32) NOT NULL,
  credit_subject VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  event_id VARCHAR(160) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_hot_reserve_j_seq (reservation_id, seq),
  KEY idx_hot_reserve_j_asset (asset, at)
);

CREATE TABLE IF NOT EXISTS hot_reserve_align (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  asset VARCHAR(16) NOT NULL,
  as_of DATETIME(3) NOT NULL,
  reservation_held VARCHAR(64) NOT NULL,
  earmarked VARCHAR(64) NOT NULL,
  consumed VARCHAR(64) NOT NULL,
  open_amt VARCHAR(64) NOT NULL,
  freeze_track VARCHAR(64) NOT NULL,
  broadcast_track VARCHAR(64) NOT NULL,
  drift TINYINT NOT NULL,
  drift_kind VARCHAR(32) NOT NULL DEFAULT '',
  ticket_id VARCHAR(64) NOT NULL DEFAULT '',
  blocks TINYINT NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  event_id VARCHAR(128) NOT NULL,
  KEY idx_hot_reserve_align_asset (asset, as_of)
);

CREATE TABLE IF NOT EXISTS hot_reserve_drift (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  asset VARCHAR(16) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  state VARCHAR(16) NOT NULL,
  reservation_held VARCHAR(64) NOT NULL,
  freeze_track VARCHAR(64) NOT NULL,
  broadcast_track VARCHAR(64) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  report_id VARCHAR(64) NOT NULL DEFAULT '',
  opened_at DATETIME(3) NOT NULL,
  closed_at DATETIME(3) NULL,
  KEY idx_hot_reserve_drift_asset (asset, state)
);

CREATE TABLE IF NOT EXISTS hot_reserve_track (
  reservation_id VARCHAR(64) NOT NULL,
  kind VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  ref VARCHAR(128) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (reservation_id, kind),
  KEY idx_hot_reserve_track_asset (asset, kind)
);

CREATE TABLE IF NOT EXISTS hot_reserve_observe (
  asset VARCHAR(16) NOT NULL,
  kind VARCHAR(16) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (asset, kind)
);

CREATE TABLE IF NOT EXISTS hot_reserve_plan (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  asset VARCHAR(16) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  noted_at DATETIME(3) NOT NULL,
  KEY idx_hot_reserve_plan_asset (asset)
);

CREATE TABLE IF NOT EXISTS hot_reserve_hint (
  id VARCHAR(128) NOT NULL PRIMARY KEY,
  asset VARCHAR(16) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  reason VARCHAR(255) NOT NULL,
  at DATETIME(3) NOT NULL,
  KEY idx_hot_reserve_hint_asset (asset, at)
);
