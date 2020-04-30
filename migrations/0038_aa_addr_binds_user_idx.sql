-- 充值地址绑定按用户+链读索引。
-- 已分配充值地址按 user_id、chain 检索。仅加性索引，不改 aa_addr_binds 列。缺该索引时用户级 List 走全表扫描，读面超时。
CREATE INDEX idx_aa_addr_binds_user ON aa_addr_binds (user_id, chain);
-- 作者：TianpengLan
