-- 子账户 API Key：创建 IP、路径前缀白名单、空闲过期 TTL。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN created_ip VARCHAR(64) NOT NULL DEFAULT '',
-- 作者：Kinsley
  ADD COLUMN path_allow VARCHAR(1024) NOT NULL DEFAULT '',
  ADD COLUMN idle_ttl BIGINT NOT NULL DEFAULT 0;
