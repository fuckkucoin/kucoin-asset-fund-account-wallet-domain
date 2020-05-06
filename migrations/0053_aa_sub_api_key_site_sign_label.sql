-- 子账户 API Key：SG/TYO 站点绑定、签名版本、运维标签、轮换计数/IP；审计操作者 IP。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN site_allow VARCHAR(16) NOT NULL DEFAULT '',
-- 作者：Axel
  ADD COLUMN sign_ver TINYINT NOT NULL DEFAULT 2,
  ADD COLUMN label VARCHAR(64) NOT NULL DEFAULT '',
  ADD COLUMN rotate_count INT NOT NULL DEFAULT 0,
  ADD COLUMN rotated_ip VARCHAR(64) NOT NULL DEFAULT '';
