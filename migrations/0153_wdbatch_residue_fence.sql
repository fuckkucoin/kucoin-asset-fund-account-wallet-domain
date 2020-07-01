-- 出金批次 Retry 残差围栏与双冻证明。
-- 作者：Beck
-- 日期：2021.12.17

CREATE TABLE IF NOT EXISTS wdbatch_residue_fences (
    id                   VARCHAR(64) PRIMARY KEY,
    batch_id             VARCHAR(64) NOT NULL,
    item_idem            VARCHAR(128) NOT NULL,
    freeze_fingerprint   VARCHAR(64) NOT NULL,
    generation           INT NOT NULL,
    ttl_ms               BIGINT NOT NULL,
    issued_at            TIMESTAMPTZ NOT NULL,
    expire_at            TIMESTAMPTZ NOT NULL,
    frozen_at            TIMESTAMPTZ,
    released_at          TIMESTAMPTZ,
    state                VARCHAR(16) NOT NULL,
    actor                VARCHAR(64) NOT NULL,
    user_id              VARCHAR(64) NOT NULL,
    asset                VARCHAR(16) NOT NULL,
    amount               VARCHAR(64) NOT NULL,
    original_freeze_ref  VARCHAR(128),
    resume_only          BOOLEAN NOT NULL DEFAULT FALSE,
    reason               VARCHAR(256)
);
CREATE INDEX IF NOT EXISTS idx_wdbatch_res_fence_item ON wdbatch_residue_fences (batch_id, item_idem);
CREATE INDEX IF NOT EXISTS idx_wdbatch_res_fence_live ON wdbatch_residue_fences (state, expire_at);

CREATE TABLE IF NOT EXISTS wdbatch_dup_freeze_proofs (
    id                VARCHAR(64) PRIMARY KEY,
    batch_id          VARCHAR(64) NOT NULL,
    item_idem         VARCHAR(128) NOT NULL,
    winner_fence_id   VARCHAR(64) NOT NULL,
    loser_attempt_id  VARCHAR(64) NOT NULL,
    loser_actor       VARCHAR(64) NOT NULL,
    reason            VARCHAR(64) NOT NULL,
    fingerprint       VARCHAR(64) NOT NULL,
    exported          BOOLEAN NOT NULL DEFAULT FALSE,
    exported_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_wdbatch_dup_proof_batch ON wdbatch_dup_freeze_proofs (batch_id, item_idem);

CREATE TABLE IF NOT EXISTS wdbatch_half_submit_cases (
    id          VARCHAR(64) PRIMARY KEY,
    batch_id    VARCHAR(64) NOT NULL,
    item_idem   VARCHAR(128) NOT NULL,
    fence_id    VARCHAR(64) NOT NULL,
    state       VARCHAR(16) NOT NULL,
    cut_at      TIMESTAMPTZ NOT NULL,
    expire_at   TIMESTAMPTZ NOT NULL,
    resume_at   TIMESTAMPTZ,
    actor       VARCHAR(64) NOT NULL,
    cut_reason  VARCHAR(256),
    full_frozen BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_wdbatch_half_open ON wdbatch_half_submit_cases (batch_id, item_idem, state);

CREATE TABLE IF NOT EXISTS wdbatch_freeze_slots (
    item_idem    VARCHAR(128) PRIMARY KEY,
    batch_id     VARCHAR(64) NOT NULL,
    fence_id     VARCHAR(64) NOT NULL,
    fingerprint  VARCHAR(64) NOT NULL,
    status       VARCHAR(16) NOT NULL,
    held_at      TIMESTAMPTZ NOT NULL,
    released_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS wdbatch_submit_retry_mutex (
    batch_id   VARCHAR(64) PRIMARY KEY,
    kind       VARCHAR(16) NOT NULL,
    holder     VARCHAR(64) NOT NULL,
    held_at    TIMESTAMPTZ NOT NULL,
    expire_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS wdbatch_residue_fence_audits (
    id         VARCHAR(64) PRIMARY KEY,
    batch_id   VARCHAR(64) NOT NULL,
    fence_id   VARCHAR(64),
    event      VARCHAR(64) NOT NULL,
    from_state VARCHAR(16),
    to_state   VARCHAR(16),
    actor      VARCHAR(64) NOT NULL,
    detail     VARCHAR(256),
    at         TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_wdbatch_res_audit_batch ON wdbatch_residue_fence_audits (batch_id, at);
