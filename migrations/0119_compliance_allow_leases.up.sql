-- 合规放行租约收口引擎 — 数据表定义。
-- compliance_allow_leases: 放行租约主表
-- compliance_allow_proofs: 漂移封堵凭证表
-- 作者：TianpengLan
-- compliance_policy_binds: 策略绑定快照表

CREATE TABLE IF NOT EXISTS compliance_allow_leases (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    movement_id     VARCHAR(128) NOT NULL,
    user_id         VARCHAR(128) NOT NULL,
    case_id         VARCHAR(128),
    asset           VARCHAR(32)  NOT NULL,
    chain           VARCHAR(64)  NOT NULL,
    to_address      VARCHAR(512) NOT NULL,
    amount          VARCHAR(64)  NOT NULL,
    decision        VARCHAR(32)  NOT NULL,
    policy_ver      INT          NOT NULL,
    state           VARCHAR(32)  NOT NULL DEFAULT 'allowed',
    ttl_seconds     INT          NOT NULL DEFAULT 300,
    issued_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ  NOT NULL,
    bound_at        TIMESTAMPTZ,
    consumed_at     TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    expired_at      TIMESTAMPTZ,
    revoke_reason   TEXT,
    bound_movement  VARCHAR(128),
    fence_epoch     BIGINT       NOT NULL DEFAULT 0,
    fence_token     VARCHAR(128),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cal_movement    ON compliance_allow_leases (movement_id);
CREATE INDEX IF NOT EXISTS idx_cal_user_state  ON compliance_allow_leases (user_id, state);
CREATE INDEX IF NOT EXISTS idx_cal_state       ON compliance_allow_leases (state) WHERE state IN ('allowed','bound');
CREATE INDEX IF NOT EXISTS idx_cal_expires     ON compliance_allow_leases (expires_at) WHERE state IN ('allowed','bound');
CREATE INDEX IF NOT EXISTS idx_cal_case        ON compliance_allow_leases (case_id) WHERE case_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS compliance_allow_proofs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lease_id        UUID REFERENCES compliance_allow_leases(id),
    movement_id     VARCHAR(128) NOT NULL,
    user_id         VARCHAR(128) NOT NULL,
    asset           VARCHAR(32),
    chain           VARCHAR(64),
    to_address      VARCHAR(512),
    amount          VARCHAR(64),
    drift_kind      VARCHAR(64)  NOT NULL,
    lease_state     VARCHAR(32),
    case_state      VARCHAR(32),
    policy_ver      INT,
    reason          TEXT         NOT NULL,
    blocked         BOOLEAN      NOT NULL DEFAULT TRUE,
    sealed          BOOLEAN      NOT NULL DEFAULT FALSE,
    sealed_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cap_movement ON compliance_allow_proofs (movement_id);
CREATE INDEX IF NOT EXISTS idx_cap_user     ON compliance_allow_proofs (user_id);
CREATE INDEX IF NOT EXISTS idx_cap_lease    ON compliance_allow_proofs (lease_id) WHERE lease_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS compliance_policy_binds (
    lease_id        UUID PRIMARY KEY REFERENCES compliance_allow_leases(id),
    policy_ver      INT          NOT NULL,
    decision        VARCHAR(32)  NOT NULL,
    case_id         VARCHAR(128),
    hook            VARCHAR(64),
    screen_score    VARCHAR(64),
    bound_at        TIMESTAMPTZ
);
