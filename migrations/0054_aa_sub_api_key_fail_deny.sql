-- 子账户 API Key：最近失败时间/IP 与拒绝计数，供运维。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN last_fail_at BIGINT NOT NULL DEFAULT 0,
-- 作者：Axel
  ADD COLUMN last_fail_ip VARCHAR(64) NOT NULL DEFAULT '',
  ADD COLUMN deny_count BIGINT NOT NULL DEFAULT 0;
