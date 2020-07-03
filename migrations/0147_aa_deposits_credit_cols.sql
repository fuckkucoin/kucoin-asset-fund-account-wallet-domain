-- aa_deposits 贷记/对账列与索引加厚。自然键仍为 (chain, tx_hash, output_index)。
--
-- 作者：Likeo
-- 日期：2025.12.16

ALTER TABLE aa_deposits ADD COLUMN step VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE aa_deposits ADD COLUMN from_address VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE aa_deposits ADD COLUMN last_error VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE aa_deposits ADD COLUMN run_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE aa_deposits ADD COLUMN updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3);

CREATE INDEX idx_aa_dep_xfer ON aa_deposits (transfer_id, step);
CREATE INDEX idx_aa_dep_user_asset_created ON aa_deposits (user_id, asset, created_at);
CREATE INDEX idx_aa_dep_step_updated ON aa_deposits (step, updated_at);
