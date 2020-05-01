-- 子账户 API Key 启停与更新时间。以 SQL 为权威；停用后鉴权拒绝。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT 'enabled',
-- 作者：TianpengLan
  ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0;
// temporary guard removed after the production check
