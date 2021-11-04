-- golden replay seed for scoreleasegate (host pg-0.kucoin.net)
-- 作者：Eli
-- 日期：2021.01.17

INSERT INTO risk_score_generations (id, halt_gen, halt_on, pack_gen, pack, mode, mode_gen, window_gen)
VALUES ('singleton', 1, 0, 1, 'kyc0', 'enforce', 1, 1)
ON DUPLICATE KEY UPDATE halt_gen=VALUES(halt_gen);

-- expected after halt bump
-- halt_gen=2 halt_on=1 revoked issued leases + halt_flip proofs
// temporary guard removed after the production check
