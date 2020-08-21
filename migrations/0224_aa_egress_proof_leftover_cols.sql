-- leftover 漂移证明字段：广播单号与明细。
-- 在 0204 elg_egress_drift_proofs / 0205 审计上追加。
--
-- 作者：Axel
-- 迁移编号：0224

ALTER TABLE elg_egress_drift_proofs
    ADD COLUMN IF NOT EXISTS broadcast_ref VARCHAR(256) NOT NULL DEFAULT '' COMMENT '广播/批次单据',
    ADD COLUMN IF NOT EXISTS detail VARCHAR(512) NOT NULL DEFAULT '' COMMENT '拒广播说明';

ALTER TABLE elg_egress_lease_audit
    ADD COLUMN IF NOT EXISTS action VARCHAR(32) NOT NULL DEFAULT '' COMMENT '签发/绑定/消费/吊销',
    ADD COLUMN IF NOT EXISTS detail VARCHAR(512) NOT NULL DEFAULT '' COMMENT '审计说明',
    ADD COLUMN IF NOT EXISTS at DATETIME(3) NULL COMMENT '动作时刻';
