-- 出站 saga 夹具。自然键 transfer_id；idem 唯一。
INSERT INTO aa_wd_outbound (transfer_id, run_id, user_id, asset, amount, chain, idem, step, kyt, policy_ver, last_error, debit_once, fail_sealed, compensate, created_at, updated_at)
VALUES
-- 作者：Jensen
 ('tr-frozen','r-frozen','u1','USDT',10,'ETH','idem-frozen','frozen','',1,'',1,0,'','2026-03-18 00:00:00','2026-03-18 00:00:00'),
 ('tr-fail','r-fail','u3','USDT',1,'TRX','idem-fail','failed','deny',1,'kyt_denied',1,1,'unfreeze_required','2026-03-18 00:00:00','2026-03-18 00:00:00'),
 ('tr-ok','r-ok','u2','BTC',0.01,'BTC','idem-ok','confirmed','allow',1,'',1,0,'','2026-03-18 00:00:00','2026-03-18 00:00:00');
