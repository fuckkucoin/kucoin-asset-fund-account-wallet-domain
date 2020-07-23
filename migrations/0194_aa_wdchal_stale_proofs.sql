-- 出金挑战旧令牌拒放行证明表（wdverify_stale_proofs）。
--
-- 设计约束：
--   - 租约内因子/组合变更（TOTP 重绑、combo_gen 抬升、冷却重置）后旧租约置 stale，同时开 stale 证明。
--   - state 生命周期：open → proven（StaleChalWorker 固化后）→ closed（管理员确认）。
--   - open 状态下，同 (user_id, old_lease_id) 的 CheckWithdraw 一律拒绝并附 proof_id。
--   - proof_hash 唯一，防重复固化同一证明（SHA-256 hex）。
--   - idem_key 唯一，防重复开证明。
--   - 隔离：wdvc_ 前缀；不含 tradepw 哈希字段；不含 seclock Until 字段；不含 wdguard 字段。
--
-- 迁移编号：0194
-- 作者：Span / Likeo / King
-- 日期：2022.09.01

CREATE TABLE IF NOT EXISTS wdvc_stale_proofs (
    id              VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    lease_id        VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '旧 wdvc_chal_leases.id（已置 stale）',
    user_id         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '受影响用户 ID',
    factor          VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '旧租约因子',
    old_combo_gen   BIGINT        NOT NULL DEFAULT 0             COMMENT '旧租约签发时的 combo_gen',
    new_combo_gen   BIGINT        NOT NULL DEFAULT 0             COMMENT '发现漂移时的当前 combo_gen',
    drift_reason    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '漂移原因（totp_rebind / combo_gen_drift / cooldown_raise）',
    state           VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open | proven | closed',
    proof_hash      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'SHA-256 hex 证明指纹（唯一）',
    idem_key        VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键（防重复开证明）',
    opened_at       DATETIME(3)   NOT NULL                       COMMENT '证明开启时刻',
    proven_at       DATETIME(3)                                  COMMENT '证明固化时刻（NULL = 未固化）',
    closed_at       DATETIME(3)                                  COMMENT '证明关闭时刻（NULL = 未关闭）',
    closed_by       VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '关闭操作人 ID',
    notes           VARCHAR(1024) NOT NULL DEFAULT ''            COMMENT '关闭备注',
    created_at      DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at      DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_wdvc_stale_proofs_hash (proof_hash),
    UNIQUE KEY uk_wdvc_stale_proofs_idem (idem_key),
    KEY idx_wdvc_stale_proofs_lease (lease_id),
    KEY idx_wdvc_stale_proofs_user_state (user_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金挑战旧令牌拒放行证明';
