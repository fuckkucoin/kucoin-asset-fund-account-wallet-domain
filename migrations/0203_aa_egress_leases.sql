-- 热油箱出金门控租约表（elg_egress_leases）。
--
-- 设计约束：
--   - CheckEgress / CheckEgressFromTanks 得到 Allow（或 Warn 且策略允许）后必须签发 egress_lease_id。
--   - 出金广播 / 批次放行提交必须携带未过期租约；缺租约 fail-closed。
--   - flags_fingerprint / tank_digest / breach_level / halt_generation 在签发时固化；
--     BroadcastAssert 复验，漂移则置 revoked 并落 EgressDriftProof。
--   - HaltAll / HaltChain / HaltAsset 或水位跌入 Halt 后，活跃租约立即吊销。
--   - 不存储 RefillMutex 代次、不存储 hotbound cap、不存储评分租约。
--
-- 状态机（FSM）：
--   issued ──(BroadcastAssert 绑定)──▶ bound
--   bound ──(复验通过)──▶ consumed                 [终态]
--   issued | bound ──(TTL 超限)──▶ expired         [终态]
--   issued | bound ──(Halt/水位漂移)──▶ revoked    [终态]
--
-- 迁移编号：0203
-- 作者：Span / Likeo / King
-- 日期：2021.11.16

CREATE TABLE IF NOT EXISTS elg_egress_leases (
    id                      VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4，egress_lease_id',
    chain                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '链标识（大写）',
    asset                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码（大写）',
    decision_at_issue       VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '签发时门控结论 allow|warn',
    breach_level            VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '签发时水位等级 ok|warn|halt',
    flags_fingerprint       VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时 Flags 指纹',
    tank_digest             VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时 Tank 摘要',
    halt_generation         BIGINT        NOT NULL DEFAULT 0             COMMENT '签发时停仓世代',
    hot_avail_at_issue      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时热账/仓位可用',
    min_at_issue            VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时下限',
    target_at_issue         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时目标',
    source                  VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT 'catalog | tank',
    state                   VARCHAR(16)   NOT NULL DEFAULT 'issued'      COMMENT 'issued | bound | consumed | expired | revoked',
    idem_key                VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键',
    broadcast_ref           VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '广播/批次单据 ID',
    issued_at               DATETIME(3)   NOT NULL                       COMMENT '签发时刻',
    expires_at              DATETIME(3)   NOT NULL                       COMMENT '过期时刻',
    bound_at                DATETIME(3)                                  COMMENT '绑定时刻',
    consumed_at             DATETIME(3)                                  COMMENT '消费时刻',
    revoked_at              DATETIME(3)                                  COMMENT '吊销时刻',
    revoke_reason           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'halt_flip / level_halt / flags_drift / tank_drift / refill_active / manual / expiry',
    drift_proof_id          VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 elg_egress_drift_proofs.id',
    created_at              DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at              DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_elg_leases_idem (idem_key),
    KEY idx_elg_leases_scope (chain, asset, state),
    KEY idx_elg_leases_expires (state, expires_at),
    KEY idx_elg_leases_gen (halt_generation, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='热油箱出金门控租约（egress_lease）';
