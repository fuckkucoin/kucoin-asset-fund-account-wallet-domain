-- 目的地在途世代互斥封堵引擎迁移。
--
-- 包含三张表：
--   dest_inflight_gens    — 在途世代租约（InflightGenLease）
--   dest_inflight_cases   — 在途漂移案例（InflightDriftCase）
--   dest_inflight_proofs  — 在途广播证明（InflightProof）
--
-- 设计原则：
--   1. dest_inflight_gens.(entry_id, movement_id) 唯一索引确保同一出金单 Acquire 幂等；
--      InnoDB gap lock 防止并发插入同一 (entry_id, movement_id)。
--   2. dest_inflight_gens.state 由应用层 CAS 控制状态机（acquired→bound→released）；
--      数据库层不添加触发器。
--   3. dest_inflight_cases.entry_id+state=open 业务唯一性由应用层保证；
--      同一 entry_id 存在 open 案例时广播路径被完全阻断，直到四眼收口。
--   4. dest_inflight_proofs.(lease_id, movement_id) 唯一索引确保同一广播证明幂等写入；
--      缺少证明记录不得当作广播已成功。
--   5. 三表均不引用 destallow 主链外键；隔离边界在应用层保证。
--   6. inflight_gen 字段存储 UnixNano 时间戳，单调递增，无全局冲突风险
--      （各 entry_id 独立签发，不存在跨 entry 竞争）。
--   7. bound_revoke_gen 存储 BindRef 时快照的 dest_revoke_gens.gen，
--      广播路径比较该值与当前 gen 确认世代一致性。
--
-- 迁移标识：0068
-- 作者：Axel
-- 日期：2021.08.15

-- +migrate Up

CREATE TABLE IF NOT EXISTS dest_inflight_gens (
    lease_id            VARCHAR(64)   NOT NULL                    COMMENT '在途世代租约 ID（UUID）',
    entry_id            VARCHAR(128)  NOT NULL                    COMMENT '目的地条目 ID（来自 destallow）',
    user_id             VARCHAR(64)   NOT NULL                    COMMENT '用户 ID',
    chain               VARCHAR(32)   NOT NULL                    COMMENT '链名（大写，如 ETH/BTC）',
    address             VARCHAR(256)  NOT NULL                    COMMENT '目的地地址',
    movement_id         VARCHAR(128)  NOT NULL                    COMMENT '出金单 ID（与广播绑定）',

    -- 世代字段
    inflight_gen        BIGINT        NOT NULL DEFAULT 0          COMMENT '在途世代（UnixNano，AcquireRef 时签发）',
    bound_revoke_gen    BIGINT        NOT NULL DEFAULT 0          COMMENT '绑定时快照的吊销世代（BindRef 时写入）',

    -- 状态
    state               VARCHAR(16)   NOT NULL DEFAULT 'acquired'
                            COMMENT 'acquired|bound|released',

    -- 时间戳
    issued_at           DATETIME(3)   NOT NULL                    COMMENT '签发时刻（UTC）',
    expires_at          DATETIME(3)   NOT NULL                    COMMENT '过期时刻（UTC）',
    bound_at            DATETIME(3)   NULL                        COMMENT '绑定广播时刻',
    released_at         DATETIME(3)   NULL                        COMMENT '释放时刻',

    -- 操作元数据
    actor               VARCHAR(128)  NOT NULL DEFAULT ''         COMMENT '操作者标识',
    note                TEXT          NULL                        COMMENT '状态变更备注',

    PRIMARY KEY (lease_id),
    UNIQUE KEY uk_digen_entry_movement (entry_id, movement_id),
    KEY idx_digen_entry_state          (entry_id, state),
    KEY idx_digen_expires              (expires_at, state),
    KEY idx_digen_state_issued         (state, issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='目的地在途世代租约（AcquireRef 签发，广播路径绑定验证）';

CREATE TABLE IF NOT EXISTS dest_revoke_gens (
    entry_id            VARCHAR(128)  NOT NULL                    COMMENT '目的地条目 ID',
    user_id             VARCHAR(64)   NOT NULL DEFAULT ''         COMMENT '用户 ID',
    chain               VARCHAR(32)   NOT NULL DEFAULT ''         COMMENT '链名',
    address             VARCHAR(256)  NOT NULL DEFAULT ''         COMMENT '目的地地址',

    -- 吊销世代（单调递增）
    gen                 BIGINT        NOT NULL DEFAULT 0          COMMENT '吊销世代（BeginRevoking 时抬升）',

    -- 状态
    state               VARCHAR(16)   NOT NULL DEFAULT 'active'
                            COMMENT 'active|revoking|revoked',
    active_refs         INT           NOT NULL DEFAULT 0          COMMENT '当前活跃引用计数',

    -- 时间戳
    initiated_at        DATETIME(3)   NOT NULL                    COMMENT '首次初始化时刻（UTC）',
    revoked_at          DATETIME(3)   NULL                        COMMENT '完成吊销时刻',

    -- 操作元数据
    actor               VARCHAR(128)  NOT NULL DEFAULT ''         COMMENT '最后操作者标识',

    PRIMARY KEY (entry_id),
    KEY idx_drevgen_state              (state, initiated_at),
    KEY idx_drevgen_user_chain         (user_id, chain, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='目的地吊销世代记录（BeginRevoking 抬升，广播路径世代一致性）';

CREATE TABLE IF NOT EXISTS dest_inflight_cases (
    case_id             VARCHAR(64)   NOT NULL                    COMMENT '漂移案例 ID（UUID）',
    entry_id            VARCHAR(128)  NOT NULL                    COMMENT '目的地条目 ID',
    user_id             VARCHAR(64)   NOT NULL DEFAULT ''         COMMENT '用户 ID',
    chain               VARCHAR(32)   NOT NULL DEFAULT ''         COMMENT '链名',
    address             VARCHAR(256)  NOT NULL DEFAULT ''         COMMENT '目的地地址',
    movement_id         VARCHAR(128)  NOT NULL DEFAULT ''         COMMENT '触发漂移的出金单 ID',
    lease_id            VARCHAR(64)   NOT NULL DEFAULT ''         COMMENT '关联在途世代租约 ID',

    -- 世代快照（发现漂移时记录）
    inflight_gen        BIGINT        NOT NULL DEFAULT 0          COMMENT '持有的在途世代',
    bound_revoke_gen    BIGINT        NOT NULL DEFAULT 0          COMMENT '绑定时的吊销世代',
    current_revoke_gen  BIGINT        NOT NULL DEFAULT 0          COMMENT '发现漂移时的当前吊销世代',
    entry_state         VARCHAR(16)   NOT NULL DEFAULT ''         COMMENT '发现漂移时的条目状态',

    -- 漂移类型
    kind                VARCHAR(64)   NOT NULL
                            COMMENT 'broadcast_after_revoke|broadcast_stale_gen|ref_leak_timeout',

    -- 案例状态
    state               VARCHAR(16)   NOT NULL DEFAULT 'open'
                            COMMENT 'open|closed',

    -- 时间戳
    opened_at           DATETIME(3)   NOT NULL                    COMMENT '案例打开时刻（UTC）',
    closed_at           DATETIME(3)   NULL                        COMMENT '案例关闭时刻',

    -- 收口元数据
    closed_by           VARCHAR(128)  NULL                        COMMENT '四眼收口操作者',
    resolution          TEXT          NULL                        COMMENT '收口决议（追回/确认/冲正）',
    note                TEXT          NULL                        COMMENT '漂移详情',

    PRIMARY KEY (case_id),
    KEY idx_dicase_entry_state         (entry_id, state),
    KEY idx_dicase_state_opened        (state, opened_at),
    KEY idx_dicase_lease               (lease_id),
    KEY idx_dicase_movement            (movement_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='目的地在途漂移案例（已 revoked 条目仍广播成功时产出，四眼收口解封）';

CREATE TABLE IF NOT EXISTS dest_inflight_proofs (
    proof_id            VARCHAR(64)   NOT NULL                    COMMENT '在途证明 ID（UUID）',
    lease_id            VARCHAR(64)   NOT NULL                    COMMENT '关联在途世代租约 ID',
    entry_id            VARCHAR(128)  NOT NULL                    COMMENT '目的地条目 ID',
    movement_id         VARCHAR(128)  NOT NULL                    COMMENT '出金单 ID',

    -- 世代绑定
    inflight_gen        BIGINT        NOT NULL DEFAULT 0          COMMENT '在途世代（与租约一致）',
    bound_revoke_gen    BIGINT        NOT NULL DEFAULT 0          COMMENT '绑定时的吊销世代',

    -- 广播信息
    broadcast_tx_id     VARCHAR(128)  NOT NULL DEFAULT ''         COMMENT '广播交易 ID（若有）',
    chain               VARCHAR(32)   NOT NULL DEFAULT ''         COMMENT '链名',
    address             VARCHAR(256)  NOT NULL DEFAULT ''         COMMENT '目的地地址',

    -- 时间戳
    created_at          DATETIME(3)   NOT NULL                    COMMENT '证明创建时刻（UTC）',

    PRIMARY KEY (proof_id),
    UNIQUE KEY uk_diproof_lease_movement (lease_id, movement_id),
    KEY idx_diproof_entry               (entry_id, created_at),
    KEY idx_diproof_movement            (movement_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='目的地在途广播证明（不可变；缺少证明不得放行广播路径）';

-- +migrate Down

DROP TABLE IF EXISTS dest_inflight_proofs;
DROP TABLE IF EXISTS dest_inflight_cases;
DROP TABLE IF EXISTS dest_revoke_gens;
DROP TABLE IF EXISTS dest_inflight_gens;
