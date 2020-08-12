-- 双控放行互斥租约表。
-- 同一请求 ID 只允许一个 worker 持有租约（holder），用于 Unlock/Execute 互斥。
-- gen（世代）随每次放行周期 CAS 递增，跨世代 Execute 一律拒绝。
--
-- 迁移编号：0180
-- 作者：Span
-- 日期：2022.03.10

CREATE TABLE IF NOT EXISTS dual_release_leases (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY,
    request_id   VARCHAR(36)  NOT NULL,
    holder       VARCHAR(128) NOT NULL DEFAULT '',
    gen          BIGINT       NOT NULL DEFAULT 0,
    state        VARCHAR(32)  NOT NULL DEFAULT 'idle',
    lease_token  VARCHAR(64)  NOT NULL DEFAULT '',
    acquired_at  DATETIME(3)  NOT NULL,
    expires_at   DATETIME(3)  NOT NULL,
    released_at  DATETIME(3)  NULL,
    fence_epoch  BIGINT       NOT NULL DEFAULT 0,
    fence_token  VARCHAR(64)  NOT NULL DEFAULT '',
    created_at   DATETIME(3)  NOT NULL,
    updated_at   DATETIME(3)  NOT NULL,
    UNIQUE KEY uk_request_id (request_id),
    KEY idx_expires_at (expires_at),
    KEY idx_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='双控放行互斥租约；gen 单调递增用于世代窗口 CAS';
