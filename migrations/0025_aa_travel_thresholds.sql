-- 旅行规则走廊门槛与 USD 牌价持久化；运营 PUT 写库后热更新内存表。
CREATE TABLE IF NOT EXISTS aa_travel_thresholds (
  corridor VARCHAR(64) NOT NULL,
-- 作者：TianpengLan
  asset VARCHAR(16) NOT NULL DEFAULT '',
  min_amount VARCHAR(64) NOT NULL DEFAULT '0',
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (corridor, asset)
);

CREATE TABLE IF NOT EXISTS aa_travel_usd_rates (
  asset VARCHAR(16) NOT NULL PRIMARY KEY,
  usd VARCHAR(64) NOT NULL,
  source VARCHAR(32) NOT NULL DEFAULT 'static',
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_travel_thresh_meta (
  k VARCHAR(32) NOT NULL PRIMARY KEY,
  v VARCHAR(64) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL
);
