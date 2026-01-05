-- 门控 Register 列探测说明（不另起表族）。
-- 必要列已存在于：
--   risk_screen_leases.expires_at / user_id
--   hold_ops_leases.expires_at / actor
--   signpipe_inflight_leases.expires_at / actor
-- 缺列须失败即关（persist.ProbeLeaseColumns），本文件不创建平行 register 表。
--
-- 作者：Likeo
-- 日期：2025.11.18

SELECT 1;
