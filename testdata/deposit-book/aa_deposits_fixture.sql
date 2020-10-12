-- 进程内夹具：自然键 (chain, tx_hash, output_index)，两 vout 不得合并。
INSERT INTO aa_deposits (chain, tx_hash, output_index, user_id, asset, transfer_id, amount, fence_epoch, fence_token, created_at, step, from_address, last_error, run_id, updated_at)
VALUES
-- 作者：Kian
('BTC', 'tx-same', 0, 'user-1', 'BTC', 'tr-a', 1.5, 1, 'tok-a', CURRENT_TIMESTAMP(3), 'accepted', 'from-1', '', 'run-a', CURRENT_TIMESTAMP(3)),
('BTC', 'tx-same', 1, 'user-1', 'BTC', 'tr-b', 0.25, 1, 'tok-b', CURRENT_TIMESTAMP(3), 'confirmed', 'from-1', '', 'run-b', CURRENT_TIMESTAMP(3)),
('ETH', 'tx-term', 0, 'user-2', 'USDT', 'tr-c', 10, 1, 'tok-c', CURRENT_TIMESTAMP(3), 'credited', '', '', 'run-c', CURRENT_TIMESTAMP(3));
