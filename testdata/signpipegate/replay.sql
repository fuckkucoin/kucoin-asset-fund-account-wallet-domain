-- 签章闸回放夹具——不接入外部库，供单测解析语句形状。
-- 作者：Kianford
-- 日期：2021.01.22

UPDATE signpipe_retry_generations
SET generation = generation + 1, updated_at = NOW(3)
WHERE movement_id = 'mv-arm-01' AND job_id = 'job-arm-01' AND generation = 0;

INSERT INTO signpipe_inflight_leases (
  id, movement_id, job_id, chain, dest, amount, canonical, nonce,
  generation, status, actor, isolation_mark, armed_at, expires_at
) VALUES (
  'lease-replay-01', 'mv-arm-01', 'job-arm-01', 'ETH', '0xabc1', 1.25,
  'mv-arm-01|ETH|0xabc1|1.250000000000000000', 'nonce-arm-01',
  1, 'armed', 'signer-a',
  'signpipe-retry-gen;not-ledger-apply;not-por;not-balproof;no-double-sign',
  NOW(3), DATE_ADD(NOW(3), INTERVAL 10 MINUTE)
);

INSERT INTO signpipe_signed_artifacts (
  id, lease_id, movement_id, job_id, canonical, sig_hex, digest, generation, isolation_mark, sealed_at
) VALUES (
  'art-replay-01', 'lease-replay-01', 'mv-se-01', 'job-se-01',
  'mv-se-01|ETH|0xabc8|8.000000000000000000', 'deadbeef',
  '00', 1,
  'signpipe-retry-gen;not-ledger-apply;not-por;not-balproof;no-double-sign',
  NOW(3)
);
