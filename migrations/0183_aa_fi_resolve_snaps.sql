-- 站内转账对手解析世代快照表（fi_resolve_snaps）。
--
-- 设计约束：
--   - 同 idem_key 下 gen 单调递增，只有一条记录处于 fresh 状态。
--   - snap_to_user_id 在快照生命周期内不可变（目录改绑不影响快照）。
--   - expires_at 到期后 patrol worker 将 state 置为 expired。
--   - 证明引用 snap_id 须与 fi_effect_proofs 外键对齐（应用层强制，不做 DB FK）。
--
-- 迁移编号：0183
-- 作者：Span / Likeo
-- 日期：2022.06.10

CREATE TABLE IF NOT EXISTS fi_resolve_snaps (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY        COMMENT 'UUID v4',
    from_user_id    VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '发起方用户 ID',
    to_user_id      VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '解析结果收款方 ID（快照锁定）',
    channel         VARCHAR(16)  NOT NULL DEFAULT ''         COMMENT '首个命中渠道：uid|email|phone',
    channels        VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '所有参与渠道（逗号分隔）',
    idem_key        VARCHAR(128) NOT NULL DEFAULT ''         COMMENT '对应 Send 幂等键',
    gen             BIGINT       NOT NULL DEFAULT 1          COMMENT '世代号，同 idem_key 下单调递增',
    asset           VARCHAR(16)  NOT NULL DEFAULT ''         COMMENT '币种',
    amount          VARCHAR(40)  NOT NULL DEFAULT '0'        COMMENT '金额（decimal string）',
    state           VARCHAR(16)  NOT NULL DEFAULT 'fresh'    COMMENT 'fresh|expired|deviated|closed',
    resolved_at     DATETIME(3)  NOT NULL                    COMMENT '解析完成时刻',
    expires_at      DATETIME(3)  NOT NULL                    COMMENT '快照过期时刻（默认 +5min）',
    deviation_at    DATETIME(3)  NULL                        COMMENT '串户检测发现时刻',
    created_at      DATETIME(3)  NOT NULL,
    updated_at      DATETIME(3)  NOT NULL,

    -- 同 idem_key 最新活跃 gen 唯一（应用层保证 fresh 唯一性）
    UNIQUE KEY uk_idem_gen (idem_key, gen),
    KEY idx_state_expires (state, expires_at),
    KEY idx_from_user     (from_user_id),
    KEY idx_to_user       (to_user_id),
    KEY idx_updated_at    (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='站内转账对手解析世代快照；fresh 有效期内不可变，到期由 patrol 置 expired';

