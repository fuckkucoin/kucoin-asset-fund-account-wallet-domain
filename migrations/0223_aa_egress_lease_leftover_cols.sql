-- leftover 出金租约字段：金额、批次、原生币、操作人。
-- 在 0203 elg_egress_leases 上追加，不改状态机，不放开无租约广播。
--
-- 作者：TianpengLan
-- 迁移编号：0223

ALTER TABLE elg_egress_leases
    ADD COLUMN IF NOT EXISTS native VARCHAR(32) NOT NULL DEFAULT '' COMMENT '原生币',
    ADD COLUMN IF NOT EXISTS amount VARCHAR(64) NOT NULL DEFAULT '' COMMENT '签发金额',
    ADD COLUMN IF NOT EXISTS batch_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '批次号',
    ADD COLUMN IF NOT EXISTS actor VARCHAR(64) NOT NULL DEFAULT '' COMMENT '签发/提交主体';
