ALTER TABLE aa_recon_tickets ADD COLUMN assignee VARCHAR(64) NOT NULL DEFAULT '';
-- 对账工单处理人。缺列则指派无法落库。
-- 作者：TianpengLan
