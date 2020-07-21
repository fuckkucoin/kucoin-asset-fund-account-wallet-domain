-- 超提屏障漂移案例表（ow_drift_cases）。
--
-- 设计约束：
--   - 每次漂移/缺 bind/过期 bind 事件均开一条 open 案例，完整记录事故轨迹。
--   - state 生命周期：open → proven（DriftPatrolWorker 固化后）→ closed（管理员确认）。
--   - open → closed 直通路径由 DismissDriftCase 触发（驳回，无需证明）。
--   - idem_key 唯一，防重复开案。
--   - blocked_at 开案即写入，表示挡写生效时刻。
--   - 隔离：ow_ 前缀；不含 availcas CAS 字段；不含 hotbound 字段；drift_epoch 键空间独立。
--
-- 迁移编号：0192
-- 作者：Span / Likeo / King
-- 日期：2022.08.10

CREATE TABLE IF NOT EXISTS ow_drift_cases (
    id             VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id        VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '受影响用户 ID',
    asset          VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '币种',
    trigger        VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT 'gen_drift | avail_drift | both_drift | no_bind | expired_bind',
    bind_id        VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 ow_precheck_binds.id（no_bind 路径为空）',
    proof_id       VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 ow_variance_proofs.id（proven 后填）',
    proof_hash     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'VarianceProof.proof_hash（防篡改）',
    state          VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open | proven | closed',
    drift_gen      BIGINT        NOT NULL DEFAULT 0             COMMENT 'LeaseGen 漂移量（commit_gen - bind_gen）',
    confirmed_by   VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '管理员 ID（closed 后填）',
    confirmed_at   DATETIME(3)                                  COMMENT '案例关闭时刻（NULL=未关闭）',
    blocked_at     DATETIME(3)   NOT NULL                       COMMENT '挡写生效时刻（开案时写入）',
    close_reason   VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '关闭原因',
    scan_note      VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '巡检器说明',
    idem_key       VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键（防重复开案）',
    drift_epoch    BIGINT        NOT NULL DEFAULT 0             COMMENT '隔离纪元（毫秒，ow_dc_ 前缀键空间）',
    created_at     DATETIME(3)   NOT NULL,
    updated_at     DATETIME(3)   NOT NULL,

    UNIQUE KEY uk_idem_key       (idem_key),
    KEY idx_state                (state),
    KEY idx_user_asset_state     (user_id, asset, state),
    KEY idx_trigger              (trigger),
    KEY idx_bind_id              (bind_id),
    KEY idx_proof_id             (proof_id),
    KEY idx_blocked_at           (blocked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='超提屏障漂移案例仓；open 时挡写；proven 待四眼；closed 解除；idem_key 防重复开案；DriftPatrolWorker 推进状态';
// temporary guard removed after the production check
