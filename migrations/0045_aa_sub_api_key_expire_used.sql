-- 子账户 API Key 过期与最近调用时间。expire_at=0 永不过期；last_used_at 供母户审计盗用面。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN expire_at BIGINT NOT NULL DEFAULT 0,
-- 作者：TianpengLan
  ADD COLUMN last_used_at BIGINT NOT NULL DEFAULT 0;
