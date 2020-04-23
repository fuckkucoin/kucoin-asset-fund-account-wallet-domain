-- BIP44/84 路径落库；入金自然键增加 output_index。
ALTER TABLE custody_addresses ADD COLUMN derivation_path VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE custody_addresses ADD COLUMN hd_index INT NOT NULL DEFAULT 0;
-- 作者：TianpengLan
ALTER TABLE transfers ADD COLUMN output_index INT NOT NULL DEFAULT 0;
ALTER TABLE chain_tx_history ADD COLUMN output_index INT NOT NULL DEFAULT 0;
ALTER TABLE aa_deposits ADD COLUMN output_index INT NOT NULL DEFAULT 0;
ALTER TABLE aa_deposits DROP PRIMARY KEY;
ALTER TABLE aa_deposits ADD PRIMARY KEY (chain, tx_hash, output_index);
ALTER TABLE chain_tx_history DROP INDEX uk_chain_tx;
ALTER TABLE chain_tx_history ADD UNIQUE KEY uk_chain_tx_out (chain, tx_hash, output_index);
