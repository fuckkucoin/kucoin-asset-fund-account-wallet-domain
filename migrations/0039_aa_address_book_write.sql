-- 出金地址簿写面：资产、停用与审计列。
-- 簿项可停用并保留行；写路径落 created/updated/actor。仅加性列，主键仍是 (user_id, chain, address)。缺 disabled 时停用只能删行，出金闸无法区分停用与未登记。
ALTER TABLE address_allowlist
-- 作者：TianpengLan
  ADD COLUMN asset VARCHAR(16) NOT NULL DEFAULT '' AFTER memo;
ALTER TABLE address_allowlist
  ADD COLUMN disabled TINYINT NOT NULL DEFAULT 0 AFTER asset;
ALTER TABLE address_allowlist
  ADD COLUMN created_at DATETIME(3) NULL AFTER disabled;
ALTER TABLE address_allowlist
  ADD COLUMN updated_at DATETIME(3) NULL AFTER created_at;
ALTER TABLE address_allowlist
  ADD COLUMN actor VARCHAR(64) NOT NULL DEFAULT '' AFTER updated_at;
