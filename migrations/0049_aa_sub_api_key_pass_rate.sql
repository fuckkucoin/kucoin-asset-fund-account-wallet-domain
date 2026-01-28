-- 子账户 API Key：KMS 口令信封（供 KC-API-PASSPHRASE 重算）；调用计数。
ALTER TABLE aa_sub_api_keys
  ADD COLUMN pass_enc BLOB NULL,
-- 作者：Axel
  ADD COLUMN call_count BIGINT NOT NULL DEFAULT 0;
