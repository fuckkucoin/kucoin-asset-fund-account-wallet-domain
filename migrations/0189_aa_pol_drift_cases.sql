-- 策略漂移案例表（ap_pol_drift_cases）及附属表。
--
-- 设计约束：
--   - idem_key 唯一（防重复开案）。
--   - proof_hash 唯一（防重复固化证明）。
--   - state 生命周期：open → proven → closed（或 open → closed，由 Dismiss 路径触发）。
--   - blocked_at 开案时即写入；挡写生效，阻断对应 BizKind 的 hop 令牌签发。
--   - 隔离：drift_epoch 前缀 pd_，与 r7 cfgcenter rollout_id、r6 D run_id、r10 G fence 完全隔离。
--
-- 附属表 ap_dual_escalates（双读升级挡写记录）：
--   - 同一 (user_id, asset, biz_kind) 最多一个 active 记录（由应用层保证）。
--   - case_id 关联 ap_pol_drift_cases；resolved 时挡写解除。
--
-- 迁移编号：0189
-- 作者：Span / Likeo / King
-- 日期：2022.07.20

CREATE TABLE IF NOT EXISTS ap_pol_drift_cases (
    id            VARCHAR(36)   NOT NULL PRIMARY KEY            COMMENT 'UUID v4',
    user_id       VARCHAR(64)   NOT NULL DEFAULT ''             COMMENT '受影响用户（空=全局）',
    asset         VARCHAR(16)   NOT NULL DEFAULT ''             COMMENT '币种（空=全资产）',
    biz_kind      VARCHAR(32)   NOT NULL DEFAULT ''             COMMENT '触发案例的业务类型',
    trigger       VARCHAR(32)   NOT NULL DEFAULT ''             COMMENT 'dual_read | unauthorized | window_race',
    trigger_gen   BIGINT        NOT NULL DEFAULT 0              COMMENT '触发时主库策略世代',
    replica_gen   BIGINT        NOT NULL DEFAULT 0              COMMENT '触发时副库策略世代（dual_read 路径有效）',
    diverge_delta BIGINT        NOT NULL DEFAULT 0              COMMENT '分歧量（trigger_gen - replica_gen）',
    hop_token_id  VARCHAR(36)   NOT NULL DEFAULT ''             COMMENT '关联越权令牌（unauthorized 触发时有效）',
    state         VARCHAR(16)   NOT NULL DEFAULT 'open'         COMMENT 'open | proven | closed',
    proof_id      VARCHAR(36)   NOT NULL DEFAULT ''             COMMENT 'ap_drift_proofs.id（proven 后填）',
    proof_hash    VARCHAR(64)   NOT NULL DEFAULT ''             COMMENT 'DriftProof.proof_hash（防篡改）',
    confirmed_by  VARCHAR(64)   NOT NULL DEFAULT ''             COMMENT '管理员 ID（closed 后填）',
    confirmed_at  DATETIME(3)                                   COMMENT '案例关闭时刻',
    blocked_at    DATETIME(3)   NOT NULL                        COMMENT '挡写生效时刻',
    close_reason  VARCHAR(512)  NOT NULL DEFAULT ''             COMMENT '关闭原因',
    scan_note     VARCHAR(512)  NOT NULL DEFAULT ''             COMMENT '扫描器说明',
    idem_key      VARCHAR(256)  NOT NULL DEFAULT ''             COMMENT '幂等键（防重复开案）',
    drift_epoch   BIGINT        NOT NULL DEFAULT 0              COMMENT '隔离纪元（毫秒，pd_ 前缀键空间）',
    created_at    DATETIME(3)   NOT NULL,
    updated_at    DATETIME(3)   NOT NULL,

    UNIQUE KEY uk_idem_key    (idem_key),
    KEY idx_state             (state),
    KEY idx_user_biz          (user_id, biz_kind),
    KEY idx_trigger_gen       (trigger_gen),
    KEY idx_blocked_at        (blocked_at),
    KEY idx_hop_token_id      (hop_token_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='策略漂移案例仓；open 时挡写；proven 需证明；closed 挡写解除；idem_key 防重复开案';

-- 漂移案例证明表（ap_drift_proofs）
CREATE TABLE IF NOT EXISTS ap_drift_proofs (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY          COMMENT 'UUID v4',
    case_id      VARCHAR(36)  NOT NULL DEFAULT ''           COMMENT 'ap_pol_drift_cases.id',
    trigger_gen  BIGINT       NOT NULL DEFAULT 0            COMMENT '主库世代快照',
    replica_gen  BIGINT       NOT NULL DEFAULT 0            COMMENT '副库世代快照',
    proof_hash   VARCHAR(64)  NOT NULL DEFAULT ''           COMMENT 'SHA-256 hex',
    sealed_at    DATETIME(3)  NOT NULL                      COMMENT '证明固化时刻',
    created_at   DATETIME(3)  NOT NULL,

    UNIQUE KEY uk_case_id    (case_id),
    UNIQUE KEY uk_proof_hash (proof_hash),
    KEY idx_sealed_at        (sealed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='策略漂移证明仓；proof_hash 防重放；case_id 唯一（每案只有一份证明）';

-- 双读升级挡写记录表（ap_dual_escalates）
CREATE TABLE IF NOT EXISTS ap_dual_escalates (
    id                VARCHAR(36)  NOT NULL PRIMARY KEY         COMMENT 'UUID v4',
    user_id           VARCHAR(64)  NOT NULL DEFAULT ''          COMMENT '空=全局；非空=用户级',
    asset             VARCHAR(16)  NOT NULL DEFAULT ''          COMMENT '空=全资产',
    biz_kind          VARCHAR(32)  NOT NULL DEFAULT ''          COMMENT '挡写的业务类型',
    threshold_count   INT          NOT NULL DEFAULT 0           COMMENT '触发升级的分歧次数',
    max_diverge_delta BIGINT       NOT NULL DEFAULT 0           COMMENT '最大分歧量（绝对值）',
    case_id           VARCHAR(36)  NOT NULL DEFAULT ''          COMMENT '关联 ap_pol_drift_cases.id',
    state             VARCHAR(16)  NOT NULL DEFAULT 'active'    COMMENT 'active | resolved',
    escalated_at      DATETIME(3)  NOT NULL                     COMMENT '升级挡写生效时刻',
    resolved_at       DATETIME(3)                               COMMENT '挡写解除时刻（NULL=未解除）',
    escalate_epoch    BIGINT       NOT NULL DEFAULT 0           COMMENT '隔离纪元（毫秒，de_ 前缀键空间）',
    created_at        DATETIME(3)  NOT NULL,
    updated_at        DATETIME(3)  NOT NULL,

    KEY idx_state              (state),
    KEY idx_user_asset_biz     (user_id, asset, biz_kind),
    KEY idx_case_id            (case_id),
    KEY idx_escalated_at       (escalated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='双读升级挡写记录；active 时阻断对应 BizKind 的令牌签发；resolved 后解除挡写';

-- PolicyVersion 表（ap_policy_versions）
-- 注：acctpol.MemStore 目前足够用于；建表供 SQLStore 迁移使用。
CREATE TABLE IF NOT EXISTS ap_policy_versions (
    id              VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '空=全局版本；非空=个人钉版',
    asset           VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '币种（空=全资产）',
    gen             BIGINT        NOT NULL DEFAULT 0             COMMENT '策略世代号（全局单调递增）',
    state           VARCHAR(16)   NOT NULL DEFAULT 'draft'       COMMENT 'draft | active | pinned | superseded',
    allow_biz_kinds VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '允许业务类型（逗号分隔）',
    deny_biz_kinds  VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '拒绝业务类型（逗号分隔）',
    require_token   TINYINT(1)    NOT NULL DEFAULT 0             COMMENT '是否强制要求 HopToken',
    max_single_amt  VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT '单笔上限（decimal string）',
    daily_limit     VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT '日限额（decimal string）',
    doc_hash        VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '内容指纹（SHA-256 hex）',
    pinned_by       VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '钉版操作人',
    pinned_at       DATETIME(3)                                  COMMENT '钉版时刻',
    published_by    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '发布操作人',
    published_at    DATETIME(3)                                  COMMENT '发布时刻',
    superseded_at   DATETIME(3)                                  COMMENT '被取代时刻',
    pol_epoch       BIGINT        NOT NULL DEFAULT 0             COMMENT '隔离纪元（毫秒，av_ 前缀键空间）',
    created_at      DATETIME(3)   NOT NULL,
    updated_at      DATETIME(3)   NOT NULL,

    UNIQUE KEY uk_gen           (gen),
    KEY idx_state               (state),
    KEY idx_user_asset          (user_id, asset),
    KEY idx_published_at        (published_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='策略版本仓；gen 全局唯一单调递增；active 状态参与 hop 评估；pinned 优先于 active';

-- DualReadLog 表（ap_dual_read_logs）
CREATE TABLE IF NOT EXISTS ap_dual_read_logs (
    id             VARCHAR(36)  NOT NULL PRIMARY KEY         COMMENT 'UUID v4',
    user_id        VARCHAR(64)  NOT NULL DEFAULT ''          COMMENT '观测用户（空=全局）',
    asset          VARCHAR(16)  NOT NULL DEFAULT ''          COMMENT '币种',
    biz_kind       VARCHAR(32)  NOT NULL DEFAULT ''          COMMENT '业务类型',
    primary_gen    BIGINT       NOT NULL DEFAULT 0           COMMENT '主库读取的策略世代',
    replica_gen    BIGINT       NOT NULL DEFAULT 0           COMMENT '副库读取的策略世代',
    diverge_delta  BIGINT       NOT NULL DEFAULT 0           COMMENT '分歧量（primary_gen - replica_gen）',
    result         VARCHAR(16)  NOT NULL DEFAULT 'match'     COMMENT 'match | diverge | prim_err | repl_err',
    observed_at    DATETIME(3)  NOT NULL                     COMMENT '观测时刻',
    trace_id       VARCHAR(128) NOT NULL DEFAULT ''          COMMENT '请求追踪 ID',
    created_at     DATETIME(3)  NOT NULL,

    KEY idx_result_biz        (result, biz_kind),
    KEY idx_observed_at       (observed_at),
    KEY idx_user_biz          (user_id, biz_kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='双读观测日志；diverge 条目超阈时触发升级挡写；不含 cfgcenter rollout_id 键空间';
