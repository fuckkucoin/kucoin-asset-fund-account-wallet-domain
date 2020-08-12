-- 归集同意指纹漂移拒执行证明表（alg_agree_drift_proofs）。
--
-- 设计约束：
--   - PlanAssert / ExecAssert 发现 bal 指纹、rule 指纹或规则版本漂移时必须落证明。
--   - 缺租约、过期租约 fail-closed 亦可落对应证明（幂等）。
--   - open 状态下持该 agree_lease_id 的 Plan/Exec 一律拒绝，禁止放腿。
--   - 不写入冷却 Mark、不写入库务库存绑定、不回写 sweeppol 规则公式参数。
--
-- 证明状态机：open → proven → closed；open 可直接驳回至 closed。
--
-- 迁移编号：0207
-- 作者：Span / Likeo / King
-- 日期：2021.10.12

CREATE TABLE IF NOT EXISTS alg_agree_drift_proofs (
    id                      VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    lease_id                VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '旧 alg_agree_leases.id，缺租约可为空',
    chain                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '链标识',
    asset                   VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码',
    drift_type              VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT 'bal_drift / rule_flip / waterline / missing_lease / expired / half_plan',
    old_bal_fingerprint     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时余额指纹',
    new_bal_fingerprint     VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '复验时余额指纹',
    old_rule_fingerprint    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时规则指纹',
    new_rule_fingerprint    VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '复验时规则指纹',
    old_rule_version        BIGINT        NOT NULL DEFAULT 0             COMMENT '签发时规则版本',
    new_rule_version        BIGINT        NOT NULL DEFAULT 0             COMMENT '复验时规则版本',
    old_shadow_bal          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时影子余额',
    new_shadow_bal          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '复验时影子余额',
    old_waterline           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '签发时水位',
    new_waterline           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '复验时水位',
    state                   VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open | proven | closed',
    proof_hash              VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'SHA-256 hex',
    idem_key                VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键',
    opened_at               DATETIME(3)   NOT NULL                       COMMENT '开启时刻',
    proven_at               DATETIME(3)                                  COMMENT '固化时刻',
    closed_at               DATETIME(3)                                  COMMENT '关闭时刻',
    closed_by               VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '关闭操作人',
    notes                   VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '关闭备注',
    created_at              DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at              DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_alg_proofs_hash (proof_hash),
    UNIQUE KEY uk_alg_proofs_idem (idem_key),
    KEY idx_alg_proofs_lease (lease_id),
    KEY idx_alg_proofs_state (state, opened_at),
    KEY idx_alg_proofs_asset (asset, drift_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='归集同意指纹漂移拒执行证明（agree_drift_proof）';
