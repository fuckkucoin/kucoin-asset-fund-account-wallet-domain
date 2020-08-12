-- 出金因子组合世代表（wdverify_combo_gens）。
--
-- 设计约束：
--   - 每次因子变更（TOTP 重绑、组合规则修改、冷却抬升）写入一条新记录，gen 单调递增。
--   - Issue 时读取当前 max(gen)；CheckWithdraw 时租约签发 gen 与当前最大 gen。
--   - gen 不可回退；change_reason 记录触发来源（totp_rebind / combo_replace / cooldown_change）。
--   - 只追加，不修改；历史记录永久保留（审计溯源）。
--   - factor_sig 记录变更后的因子绑定指纹（与 wdvc_chal_leases.factor_sig 基准）。
--   - 隔离：wdvc_ 前缀；不含 tradepw pw_gen（r13 B 隔离）；不含 seclock mark（r13 E 隔离）。
--
-- 迁移编号：0195
-- 作者：Span / Likeo / King
-- 日期：2022.09.01

CREATE TABLE IF NOT EXISTS wdvc_combo_gens (
    id             VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id        VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    gen            BIGINT        NOT NULL DEFAULT 0             COMMENT '因子组合世代（单调递增，用户维度）',
    change_reason  VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '变更原因（totp_rebind / combo_replace / cooldown_change / faillock_unlock）',
    factor_sig     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '变更后因子绑定指纹 SHA-256 hex',
    actor          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '发起变更的操作人/系统标识',
    ref_id         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '关联外部单据 ID（如 TOTP 绑定 ID；可为空）',
    changed_at     DATETIME(3)   NOT NULL                       COMMENT '世代变更时刻',
    created_at     DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    KEY idx_wdvc_combo_gens_user_gen (user_id, gen),
    KEY idx_wdvc_combo_gens_changed (user_id, changed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金因子组合世代变更历史';
