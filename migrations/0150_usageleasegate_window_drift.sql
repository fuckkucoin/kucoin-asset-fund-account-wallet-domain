-- 占用窗口租约、预约、漂移证明与审计
-- 作者：King
-- 日期：2021.12.16

CREATE TABLE IF NOT EXISTS usage_leases (
  id                VARCHAR(64)  NOT NULL PRIMARY KEY,
  occ_kind          VARCHAR(16)  NOT NULL,
  user_id           VARCHAR(64)  NOT NULL,
  asset             VARCHAR(32)  NOT NULL,
  dest              VARCHAR(128) NOT NULL DEFAULT '',
  amount            VARCHAR(64)  NOT NULL,
  today_sum         VARCHAR(64)  NOT NULL,
  velocity_1h       INT          NOT NULL DEFAULT 0,
  velocity_24h      INT          NOT NULL DEFAULT 0,
  dest_count        INT          NOT NULL DEFAULT 0,
  daily_cap         VARCHAR(64)  NOT NULL,
  velocity_cap      INT          NOT NULL DEFAULT 0,
  window_print      VARCHAR(64)  NOT NULL,
  cap_print         VARCHAR(64)  NOT NULL,
  state             VARCHAR(16)  NOT NULL DEFAULT 'snapshotted',
  ttl_ms            BIGINT       NOT NULL DEFAULT 30000,
  issued_at         DATETIME(3)  NOT NULL,
  expires_at        DATETIME(3)  NOT NULL,
  reserved_at       DATETIME(3)  NULL,
  committed_at      DATETIME(3)  NULL,
  expired_at        DATETIME(3)  NULL,
  revoked_at        DATETIME(3)  NULL,
  reservation_id    VARCHAR(64)  NOT NULL DEFAULT '',
  reservation_seq   BIGINT       NOT NULL DEFAULT 0,
  proof_id          VARCHAR(64)  NOT NULL DEFAULT '',
  hit_id            VARCHAR(64)  NOT NULL DEFAULT '',
  idem              VARCHAR(64)  NOT NULL DEFAULT '',
  revoke_reason     VARCHAR(128) NOT NULL DEFAULT '',
  body              JSON         NOT NULL,
  created_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_ul_user ON usage_leases (user_id);
CREATE INDEX idx_ul_state ON usage_leases (state);
CREATE INDEX idx_ul_window ON usage_leases (occ_kind, user_id, asset, dest);
CREATE INDEX idx_ul_expires ON usage_leases (expires_at);
CREATE INDEX idx_ul_idem ON usage_leases (idem);

CREATE TABLE IF NOT EXISTS usage_reservations (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  lease_id     VARCHAR(64)  NOT NULL,
  window_key   VARCHAR(256) NOT NULL,
  occ_kind     VARCHAR(16)  NOT NULL,
  user_id      VARCHAR(64)  NOT NULL,
  asset        VARCHAR(32)  NOT NULL,
  dest         VARCHAR(128) NOT NULL DEFAULT '',
  amount       VARCHAR(64)  NOT NULL,
  state        VARCHAR(16)  NOT NULL DEFAULT 'held',
  seq          BIGINT       NOT NULL DEFAULT 0,
  held_at      DATETIME(3)  NOT NULL,
  expires_at   DATETIME(3)  NOT NULL,
  released_at  DATETIME(3)  NULL,
  consumed_at  DATETIME(3)  NULL,
  reason       VARCHAR(128) NOT NULL DEFAULT '',
  body         JSON         NOT NULL,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_ur_lease ON usage_reservations (lease_id);
CREATE INDEX idx_ur_window ON usage_reservations (window_key, state);
CREATE INDEX idx_ur_expires ON usage_reservations (expires_at);

CREATE TABLE IF NOT EXISTS usage_drift_proofs (
  id               VARCHAR(64)  NOT NULL PRIMARY KEY,
  lease_id         VARCHAR(64)  NOT NULL,
  user_id          VARCHAR(64)  NOT NULL,
  kind             VARCHAR(32)  NOT NULL,
  occ_kind         VARCHAR(16)  NOT NULL,
  asset            VARCHAR(32)  NOT NULL,
  dest             VARCHAR(128) NOT NULL DEFAULT '',
  lease_today      VARCHAR(64)  NOT NULL,
  live_today       VARCHAR(64)  NOT NULL,
  lease_print      VARCHAR(64)  NOT NULL,
  live_print       VARCHAR(64)  NOT NULL,
  lease_cap        VARCHAR(64)  NOT NULL,
  live_cap         VARCHAR(64)  NOT NULL,
  attempt_amount   VARCHAR(64)  NOT NULL,
  reserved_amount  VARCHAR(64)  NOT NULL,
  would_exceed     TINYINT      NOT NULL DEFAULT 0,
  blocked          TINYINT      NOT NULL DEFAULT 1,
  sealed           TINYINT      NOT NULL DEFAULT 1,
  detail           VARCHAR(255) NOT NULL DEFAULT '',
  body             JSON         NOT NULL,
  created_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_udp_lease ON usage_drift_proofs (lease_id);
CREATE INDEX idx_udp_user ON usage_drift_proofs (user_id);
CREATE INDEX idx_udp_kind ON usage_drift_proofs (kind);

CREATE TABLE IF NOT EXISTS usage_lease_audit (
  id          VARCHAR(64)  NOT NULL PRIMARY KEY,
  lease_id    VARCHAR(64)  NOT NULL,
  event       VARCHAR(32)  NOT NULL,
  from_state  VARCHAR(16)  NOT NULL DEFAULT '',
  to_state    VARCHAR(16)  NOT NULL DEFAULT '',
  actor       VARCHAR(64)  NOT NULL DEFAULT '',
  detail      VARCHAR(255) NOT NULL DEFAULT '',
  body        JSON         NOT NULL,
  at          DATETIME(3)  NOT NULL
);

CREATE INDEX idx_ula_lease ON usage_lease_audit (lease_id);
