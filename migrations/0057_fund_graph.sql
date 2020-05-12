-- 资金因果图。节点、有向边、摄取游标、巡检单、冲突审计。
-- 写入只落这些表。信封号只作边属性，不改源资金状态机。

-- 作者：Kinsley
CREATE TABLE IF NOT EXISTS fund_graph_nodes (
  id VARCHAR(160) NOT NULL PRIMARY KEY,
  kind VARCHAR(64) NOT NULL,
  ref VARCHAR(128) NOT NULL,
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  asset VARCHAR(32) NOT NULL DEFAULT '',
  amount VARCHAR(64) NOT NULL DEFAULT '',
  chain VARCHAR(32) NOT NULL DEFAULT '',
  tx_hash VARCHAR(128) NOT NULL DEFAULT '',
  source VARCHAR(64) NOT NULL DEFAULT '',
  occurred_at DATETIME(3) NOT NULL,
  ingested_at DATETIME(3) NOT NULL,
  attrs_json TEXT NOT NULL,
  KEY idx_fg_node_ref (ref),
  KEY idx_fg_node_user (user_id, asset, occurred_at),
  KEY idx_fg_node_kind (kind, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS fund_graph_edges (
  id VARCHAR(96) NOT NULL PRIMARY KEY,
  from_id VARCHAR(160) NOT NULL,
  amount VARCHAR(64) NOT NULL DEFAULT '',
  source VARCHAR(64) NOT NULL DEFAULT '',
  envelope_id VARCHAR(96) NOT NULL DEFAULT '',
  occurred_at DATETIME(3) NOT NULL,
  ingested_at DATETIME(3) NOT NULL,
  attrs_json TEXT NOT NULL,
  UNIQUE KEY uk_fg_edge_key (from_id, to_id, edge_type, ref),
  KEY idx_fg_edge_from (from_id, edge_type),
  KEY idx_fg_edge_to (to_id, edge_type),
  KEY idx_fg_edge_ref (ref),
  KEY idx_fg_edge_user (user_id, asset, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS fund_graph_cursors (
  source VARCHAR(64) NOT NULL,
  stream VARCHAR(64) NOT NULL,
  cursor_token VARCHAR(256) NOT NULL DEFAULT '',
  last_event_at DATETIME(3) NULL,
  updated_at DATETIME(3) NOT NULL,
  lag_ms BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (source, stream)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS fund_graph_incidents (
  id VARCHAR(96) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  edge_key VARCHAR(320) NOT NULL DEFAULT '',
  node_id VARCHAR(160) NOT NULL DEFAULT '',
  ref VARCHAR(128) NOT NULL DEFAULT '',
  detail TEXT NOT NULL,
  opened_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_fg_inc_status (status, kind),
  KEY idx_fg_inc_ref (ref)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS fund_graph_audit (
  id VARCHAR(96) NOT NULL PRIMARY KEY,
  action VARCHAR(32) NOT NULL,
  edge_key VARCHAR(320) NOT NULL DEFAULT '',
  node_id VARCHAR(160) NOT NULL DEFAULT '',
  detail TEXT NOT NULL,
  at DATETIME(3) NOT NULL,
  KEY idx_fg_audit_key (edge_key, at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
