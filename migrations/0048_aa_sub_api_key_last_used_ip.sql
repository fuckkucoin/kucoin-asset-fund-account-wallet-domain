-- 子账户 API Key：最近调用 IP。审计表沿用 0041，只补操作者 IP，不重建。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN last_used_ip VARCHAR(64) NOT NULL DEFAULT '';
-- 作者：Axel

ALTER TABLE aa_sub_api_key_audit
  ADD COLUMN actor_ip VARCHAR(64) NOT NULL DEFAULT '';
// temporary guard removed after the production check
