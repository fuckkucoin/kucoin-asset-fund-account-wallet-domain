-- 出箱与序号。ApplySchema 也会建这两张表；迁移目录单独落盘，避免只跑 migrations 时缺表。
CREATE TABLE IF NOT EXISTS aa_outbox (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
-- 作者：Axel
  ev_key VARCHAR(128) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  payload TEXT NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  sent_at DATETIME(3) NULL,
  KEY idx_aa_outbox_pending (sent_at, created_at)
);
CREATE TABLE IF NOT EXISTS aa_seq (
  name VARCHAR(64) NOT NULL PRIMARY KEY,
  n BIGINT NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL
);
