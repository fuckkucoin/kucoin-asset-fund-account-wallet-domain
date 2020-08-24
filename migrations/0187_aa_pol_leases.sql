-- 策略发布互斥租约表（ap_pol_leases）。
--
-- 设计约束：
--   - 同一 (user_id, asset) 键同一时刻至多一个 acquired 租约（由应用层保证，表无唯一约束）。
--   - holder 持有租约期间，Gate.Check 对同 (user_id, asset) 的 hop 令牌签发需等待或拒绝。
--   - policy_gen 在租约释放后写入实际完成的策略世代号；released 之前为 0。
--   - lease_epoch（毫秒时间戳）用于与 fi_debit_leases / r6 D run_id 键空间隔离；
--     前缀 ap_，不使用 r10 G / r11 G 的 fence 格式。
--
-- 迁移编号：0187
-- 作者：Span / King
-- 日期：2022.07.20

CREATE TABLE IF NOT EXISTS ap_pol_leases (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY         COMMENT 'UUID v4',
    user_id      VARCHAR(64)  NOT NULL DEFAULT ''          COMMENT '空=全局租约；非空=个人 Pin 操作租约',
    asset        VARCHAR(16)  NOT NULL DEFAULT ''          COMMENT '币种（空=全资产）',
    holder       VARCHAR(64)  NOT NULL DEFAULT ''          COMMENT '租约持有人（操作人 ID）',
    state        VARCHAR(16)  NOT NULL DEFAULT 'acquired'  COMMENT 'acquired | released | expired',
    policy_gen   BIGINT       NOT NULL DEFAULT 0           COMMENT '发布完成后的策略世代号（released 后写入）',
    acquired_at  DATETIME(3)  NOT NULL                     COMMENT '租约获取时刻',
    expires_at   DATETIME(3)  NOT NULL                     COMMENT '租约过期时刻（PublishWindowWorker 清理）',
    released_at  DATETIME(3)                               COMMENT '主动释放时刻（NULL=未释放）',
    lease_epoch  BIGINT       NOT NULL DEFAULT 0           COMMENT '隔离纪元（毫秒，ap_ 前缀键空间，≠r6 D run_id / r10 G fence）',
    created_at   DATETIME(3)  NOT NULL,
    updated_at   DATETIME(3)  NOT NULL,

    KEY idx_state_expires   (state, expires_at),
    KEY idx_user_asset      (user_id, asset),
    KEY idx_holder          (holder),
    KEY idx_acquired_at     (acquired_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='策略发布互斥租约；Publish/Pin 前获取，完成后释放；PublishWindowWorker 清理超 TTL 租约';
