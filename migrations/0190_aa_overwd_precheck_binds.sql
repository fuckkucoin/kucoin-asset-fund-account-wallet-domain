-- 超提屏障预检绑定租约表（ow_precheck_binds）。
--
-- 设计约束：
--   - PreCheck（dry-run）通过后必须落 PrecheckBind，TTL 内未 Commit 则被 BindExpiryWorker 吊销。
--   - Commit 时必须携带 bind_id；缺少 bind_id 或 bind 过期/吊销 → 拒绝放行。
--   - lease_gen 与 expected_avail 在 PreCheck 时锁定；Commit 时复验当前值与绑定时是否一致。
--   - bind_hash 防篡改（SHA-256 hex，由 Engine 写入，SQL 层不做约束）。
--   - idem_key 唯一，防重复绑定同一出金请求。
--   - 隔离：ow_ 前缀，与 ap_pol_leases（r12）/ fi_debit_leases（r12）/ de_ 键空间完全隔离。
--   - 不存储 availcas CAS 公式参数；不存储 hotbound cap 边界；仅存储只读观测快照。
--
-- 迁移编号：0190
-- 作者：Span / Likeo / King
-- 日期：2022.08.10

CREATE TABLE IF NOT EXISTS ow_precheck_binds (
    id             VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id        VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '发起用户 ID',
    asset          VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '币种',
    amount         VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT '出金请求金额（decimal string）',
    lease_gen      BIGINT        NOT NULL DEFAULT 0             COMMENT 'PreCheck 时观测的 availcas LeaseGen（只读快照）',
    expected_avail VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT 'PreCheck 时观测的 Available（decimal string，只读快照）',
    state          VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open | committed | expired | revoked',
    bind_hash      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'SHA-256 hex 内容指纹（user_id:asset:lease_gen:avail:precheck_unix）',
    idem_key       VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键（防重复 PreCheck 绑定）',
    precheck_at    DATETIME(3)   NOT NULL                       COMMENT 'PreCheck 通过时刻',
    expires_at     DATETIME(3)   NOT NULL                       COMMENT '绑定过期时刻（BindExpiryWorker 清理超 TTL）',
    committed_at   DATETIME(3)                                  COMMENT 'Commit 完成时刻（NULL=未提交）',
    commit_ref     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '实际 Commit 单据 ID（出金 ID 等）',
    revoke_reason  VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '吊销原因（漂移/过期/手动）',
    proof_id       VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 ow_variance_proofs.id（漂移路径）',
    bind_epoch     BIGINT        NOT NULL DEFAULT 0             COMMENT '隔离纪元（毫秒，ow_ 前缀键空间）',
    created_at     DATETIME(3)   NOT NULL,
    updated_at     DATETIME(3)   NOT NULL,

    UNIQUE KEY uk_idem_key      (idem_key),
    KEY idx_state_expires       (state, expires_at),
    KEY idx_user_asset_state    (user_id, asset, state),
    KEY idx_lease_gen           (lease_gen),
    KEY idx_precheck_at         (precheck_at),
    KEY idx_proof_id            (proof_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='超提屏障预检绑定租约；PreCheck 通过后创建，Commit 时复验 LeaseGen/Available；BindExpiryWorker 清理超 TTL open bind';
