-- 双控票证老化审计表。
-- 记录票证（dualctl Request）超 TTL 或 epoch 过期时的老化事件；拒放行的原因留档。
-- ticket_id 关联 aa_dualctl_requests.id；audit_hash 为 ticket+过期原因+时间戳的 SHA-256。
--
-- 迁移编号：0181
-- 作者：Likeo
-- 日期：2022.03.11

CREATE TABLE IF NOT EXISTS dual_ticket_expiries (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY,
    ticket_id    VARCHAR(36)  NOT NULL,
    request_id   VARCHAR(36)  NOT NULL,
    reason       VARCHAR(64)  NOT NULL DEFAULT 'ttl_exceeded',
    epoch_at     BIGINT       NOT NULL DEFAULT 0,
    ttl_secs     INT          NOT NULL DEFAULT 0,
    expired_at   DATETIME(3)  NOT NULL,
    actor        VARCHAR(128) NOT NULL DEFAULT '',
    audit_hash   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at   DATETIME(3)  NOT NULL,
    KEY idx_ticket_id (ticket_id),
    KEY idx_request_id (request_id),
    KEY idx_expired_at (expired_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='票证老化审计；过期拒放行的原因与时间戳留档，不可覆写';
