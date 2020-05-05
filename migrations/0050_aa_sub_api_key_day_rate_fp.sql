-- 子账户 API Key：日调用配额、单 Key 限流覆盖、secret 指纹、轮换宽限旧密文。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN day_count BIGINT NOT NULL DEFAULT 0,
-- 作者：Kinsley
  ADD COLUMN day_stamp INT NOT NULL DEFAULT 0,
  ADD COLUMN day_cap INT NOT NULL DEFAULT 0,
  ADD COLUMN rate_max INT NOT NULL DEFAULT 0,
  ADD COLUMN secret_fp VARCHAR(64) NOT NULL DEFAULT '',
  ADD COLUMN prev_secret_enc BLOB NULL,
  ADD COLUMN grace_until BIGINT NOT NULL DEFAULT 0;
