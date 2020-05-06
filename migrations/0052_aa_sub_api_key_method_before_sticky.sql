-- 子账户 API Key：HTTP 方法白名单、生效起点、创建 IP 粘滞。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN method_allow VARCHAR(64) NOT NULL DEFAULT '',
-- 作者：TianpengLan
  ADD COLUMN not_before BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN sticky_ip TINYINT NOT NULL DEFAULT 0;
