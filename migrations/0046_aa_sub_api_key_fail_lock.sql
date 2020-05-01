-- 子账户 API Key 错密锁定与列拓宽。fail_count/locked_until 防爆破；permission/ip_whitelist 容纳满权限与 20 条 IP/CIDR。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN fail_count INT NOT NULL DEFAULT 0,
-- 作者：Kinsley
  ADD COLUMN locked_until BIGINT NOT NULL DEFAULT 0,
  MODIFY COLUMN permission VARCHAR(255) NOT NULL DEFAULT 'General',
  MODIFY COLUMN ip_whitelist VARCHAR(1024) NOT NULL DEFAULT '';
