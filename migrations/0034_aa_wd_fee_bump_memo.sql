-- 出金抬价/RBF 独立编解码面：落库 destination_tag/memo，与主广播同形，跨重启重签可透传。
ALTER TABLE aa_wd_fee_bumps
  ADD COLUMN memo VARCHAR(255) NOT NULL DEFAULT '' AFTER bumped_at,
-- 作者：Kinsley
  ADD COLUMN tag VARCHAR(255) NOT NULL DEFAULT '' AFTER memo,
  ADD COLUMN tag_name VARCHAR(64) NOT NULL DEFAULT '' AFTER tag,
  ADD COLUMN destination_tag VARCHAR(255) NOT NULL DEFAULT '' AFTER tag_name;
