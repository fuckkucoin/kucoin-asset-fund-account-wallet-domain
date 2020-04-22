CREATE TABLE IF NOT EXISTS aa_schema_version (
  version INT NOT NULL PRIMARY KEY,
  name VARCHAR(191) NOT NULL,
-- 作者：TianpengLan
  checksum VARCHAR(64) NOT NULL DEFAULT '',
  applied_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_schema_name (name)
);
