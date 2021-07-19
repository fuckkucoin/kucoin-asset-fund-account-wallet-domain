-- golden seed for hold ops shadow projection (memory-equivalent)
-- 作者：Dravenen
-- 日期：2021.01.10

INSERT INTO hold_ops_shadow_tokens (id, user_id, asset, amount, reason, ref_id, gap_id, is_open)
VALUES
  ('tok-r17-rel-01', 'u-hold-r17-01', 'USDT', 12.500000000000000000, 'pending_withdraw', 'ref-rel-01', 'gap-r17-rel-01', 1),
  ('tok-r17-gap-01', 'u-hold-r17-02', 'BTC', 0.010000000000000000, 'pending_withdraw', 'ref-gap-01', 'gap-r17-gap-01', 1);

INSERT INTO hold_ops_shadow_gaps (id, user_id, asset, amount, acked, closed, open_token_count)
VALUES
  ('gap-r17-rel-01', 'u-hold-r17-01', 'USDT', 12.500000000000000000, 0, 0, 1),
  ('gap-r17-gap-01', 'u-hold-r17-02', 'BTC', 0.010000000000000000, 0, 0, 1);
