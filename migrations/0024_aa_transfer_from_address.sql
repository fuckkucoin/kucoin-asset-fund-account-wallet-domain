-- 入金付款方地址提示：充值历史与 KYT/旅行规则回显。
ALTER TABLE transfers ADD COLUMN from_address_hint VARCHAR(128) NOT NULL DEFAULT '';
-- 作者：Axel
