-- 归集读闸同意租约表（alg_agree_leases）。
--
-- 设计约束：
--   - sweeprun AgreeGate 瞬时读取影子余额与规则水位，同意码为空才签发 agree_lease_id。
--   - 签发时固化 asset、bal 指纹、rule 指纹、kinds、规则版本与 TTL。
--   - Plan 落库与 Exec 腿推进必须携带未过期租约并复验 bal+rule 指纹。
--   - 余额跌破水位或规则版本抬升后，活跃租约立即吊销，禁止继续 appendLink/放腿。
--   - 不存储 coolmarkgate 冷却 Mark 代次，不存储 trexecgate 库存绑定，不改写 sweeppol 公式。
--
-- 状态机（FSM）：
--   agreed ──(PlanAssert)──▶ planned
--   planned ──(ExecAssert)──▶ executing ──(末腿消费)──▶ consumed
--   agreed | planned | executing ──(TTL)──▶ expired
--   agreed | planned | executing ──(bal/rule 漂移)──▶ revoked
--
-- 迁移编号：0206
-- 作者：Span / Likeo / King
-- 日期：2021.10.08

CREATE TABLE IF NOT EXISTS alg_agree_leases (
    id                      VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4，agree_lease_id',
    chain                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '链标识（大写）',
    asset                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码（大写）',
    agree_code              VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时同意码，空串表示通过',
    bal_fingerprint         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时影子余额指纹',
    rule_fingerprint        VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时规则水位指纹',
    kinds                   VARCHAR(128)  NOT NULL DEFAULT ''            COMMENT '归集种类集合，逗号分隔',
    rule_version            BIGINT        NOT NULL DEFAULT 0             COMMENT '签发时规则版本',
    shadow_bal_at_issue     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时影子余额',
    waterline_at_issue      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时规则水位',
    min_sweep_at_issue      VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时最小归集额快照（只读）',
    state                   VARCHAR(16)   NOT NULL DEFAULT 'agreed'      COMMENT 'agreed|planned|executing|consumed|expired|revoked',
    idem_key                VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键',
    plan_ref                VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '计划单据 ID',
    exec_ref                VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '执行腿/作业 ID',
    issued_at               DATETIME(3)   NOT NULL                       COMMENT '签发时刻',
    expires_at              DATETIME(3)   NOT NULL                       COMMENT '过期时刻',
    planned_at              DATETIME(3)                                  COMMENT '计划绑定时刻',
    executing_at            DATETIME(3)                                  COMMENT '执行推进时刻',
    consumed_at             DATETIME(3)                                  COMMENT '消费时刻',
    revoked_at              DATETIME(3)                                  COMMENT '吊销时刻',
    revoke_reason           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'bal_drift / rule_flip / expired / missing_lease / manual',
    drift_proof_id          VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联 alg_agree_drift_proofs.id',
    created_at              DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at              DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_alg_leases_idem (idem_key),
    KEY idx_alg_leases_scope (asset, chain, state),
    KEY idx_alg_leases_expires (state, expires_at),
    KEY idx_alg_leases_rulever (rule_version, state),
    KEY idx_alg_leases_plan (plan_ref)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='归集读闸同意租约（agree_lease）';
