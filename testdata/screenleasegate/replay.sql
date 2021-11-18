-- 筛查租约事故回放夹具。
-- 作者：Capson
-- 日期：2021.01.05

INSERT INTO risk_screen_generations (id, halt_gen, halt_on, pack_gen, pack, mode)
VALUES ('singleton', 1, 0, 1, 'kyc0', 'enforce')
ON DUPLICATE KEY UPDATE halt_gen=VALUES(halt_gen);

-- 期望：Allow 后 halt_gen 抬升，旧 lease 不得 Submit。
-- 期望：Allow 后 pack_gen 抬升，旧 lease 不得 Submit。
-- 期望：缺 screen_lease_id 的 Submit 失败即关并落 missing_lease。
// temporary guard removed after the production check
