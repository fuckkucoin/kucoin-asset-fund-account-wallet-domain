-- 出金多因子挑战租约表（wdverify_chal_leases）。
--
-- 设计约束：
--   - Issue 挑战后必须落 ChallengeLease；TTL 内未消费则被 LeaseExpiryWorker 标记 expired。
--   - CheckWithdraw 必须携带 chal_lease_id；缺少 ID 或 lease 过期/旧令牌 → 拒绝放行。
--   - combo_gen 记录签发时的因子组合世代；CheckWithdraw 时与当前世代，漂移则置 stale。
--   - factor_sig 为因子绑定指纹（SHA-256 hex），签发时固化；CheckWithdraw 时复验。
--   - idem_key 唯一，防重复签发同一请求。
--   - dest_addr / dest_chain 可选，签发时绑定目标地址；校验时强制。
--   - 隔离：wdvc_ 前缀，与 ow_（r13 G）/ ap_（r12 G）/ fi_（r12 G）键空间完全隔离。
--   - 不存储 tradepw 哈希主路径参数；不存储 seclock Mark 时间戳；不存储 wdguard 三联闸票证。
--
-- 迁移编号：0193
-- 作者：Span / Likeo / King
-- 日期：2022.09.01

CREATE TABLE IF NOT EXISTS wdvc_chal_leases (
    id             VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4，chal_lease_id',
    user_id        VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '发起用户 ID',
    factor         VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '触发因子（totp / email / sms）',
    combo_gen      BIGINT        NOT NULL DEFAULT 0             COMMENT '签发时因子组合世代（ComboGeneration.gen）',
    factor_sig     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '因子绑定指纹 SHA-256 hex（签发时固化，CheckWithdraw 复验）',
    state          VARCHAR(16)   NOT NULL DEFAULT 'issued'      COMMENT 'issued | presented | consumed | expired | stale | revoked',
    dest_chain     VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '绑定目标链（可选；非空时 CheckWithdraw 强制）',
    dest_addr      VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '绑定目标地址（可选；非空时 CheckWithdraw 强制）',
    idem_key       VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键（防重复签发）',
    issued_at      DATETIME(3)   NOT NULL                       COMMENT '租约签发时刻',
    expires_at     DATETIME(3)   NOT NULL                       COMMENT '租约过期时刻（LeaseExpiryWorker 清理超 TTL）',
    presented_at   DATETIME(3)                                  COMMENT '首次提交核验时刻（NULL = 尚未提交）',
    consumed_at    DATETIME(3)                                  COMMENT '消费完成时刻（NULL = 未消费）',
    stale_at       DATETIME(3)                                  COMMENT '因子漂移置 stale 时刻（NULL = 未漂移）',
    revoke_reason  VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '吊销/stale 原因（因子变更 / 过期 / 手动）',
    proof_id       VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 wdvc_stale_proofs.id（stale 路径写入）',
    created_at     DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at     DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_wdvc_chal_leases_idem (idem_key),
    KEY idx_wdvc_chal_leases_user (user_id, state),
    KEY idx_wdvc_chal_leases_expires (state, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金多因子挑战租约';
