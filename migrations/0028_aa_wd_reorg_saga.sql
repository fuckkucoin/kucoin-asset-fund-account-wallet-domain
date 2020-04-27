-- 出金广播重组→重提 saga 持久化；跨重启可续跑。
ALTER TABLE aa_wd_broadcasts ADD COLUMN saga_phase VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE aa_wd_broadcasts ADD COLUMN old_tx_hash VARCHAR(128) NOT NULL DEFAULT '';
-- 作者：Axel
ALTER TABLE aa_wd_broadcasts ADD COLUMN resubmit_n INT NOT NULL DEFAULT 0;
ALTER TABLE aa_wd_broadcasts ADD COLUMN saga_note VARCHAR(256) NOT NULL DEFAULT '';
