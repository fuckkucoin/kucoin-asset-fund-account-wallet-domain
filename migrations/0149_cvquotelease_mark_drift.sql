-- 闪兑报价租约与牌价世代、漂移证明
-- 作者：King
-- 日期：2021.11.22

CREATE TABLE IF NOT EXISTS cv_quote_leases (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  quote_id        VARCHAR(64)  NOT NULL,
  user_id         VARCHAR(64)  NOT NULL,
  from_asset      VARCHAR(32)  NOT NULL,
  to_asset        VARCHAR(32)  NOT NULL,
  amount          VARCHAR(64)  NOT NULL,
  rate            VARCHAR(64)  NOT NULL,
  book            VARCHAR(16)  NOT NULL,
  mark_gen        BIGINT       NOT NULL DEFAULT 1,
  fingerprint     VARCHAR(64)  NOT NULL,
  book_print      VARCHAR(64)  NOT NULL DEFAULT '',
  state           VARCHAR(16)  NOT NULL DEFAULT 'issued',
  ttl_ms          BIGINT       NOT NULL DEFAULT 15000,
  issued_at       DATETIME(3)  NOT NULL,
  expires_at      DATETIME(3)  NOT NULL,
  consumed_at     DATETIME(3)  NULL,
  expired_at      DATETIME(3)  NULL,
  revoked_at      DATETIME(3)  NULL,
  used            TINYINT      NOT NULL DEFAULT 0,
  order_id        VARCHAR(64)  NOT NULL DEFAULT '',
  proof_id        VARCHAR(64)  NOT NULL DEFAULT '',
  revoke_reason   VARCHAR(128) NOT NULL DEFAULT '',
  consume_token   VARCHAR(64)  NOT NULL DEFAULT '',
  body            JSON         NOT NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_cql_quote ON cv_quote_leases (quote_id);
CREATE INDEX idx_cql_user ON cv_quote_leases (user_id);
CREATE INDEX idx_cql_state ON cv_quote_leases (state);
CREATE INDEX idx_cql_pair ON cv_quote_leases (from_asset, to_asset, state);
CREATE INDEX idx_cql_expires ON cv_quote_leases (expires_at);

CREATE TABLE IF NOT EXISTS cv_mark_gens (
  pair_key      VARCHAR(80)  NOT NULL PRIMARY KEY,
  from_asset    VARCHAR(32)  NOT NULL,
  to_asset      VARCHAR(32)  NOT NULL,
  mark_gen      BIGINT       NOT NULL DEFAULT 1,
  rate          VARCHAR(64)  NOT NULL,
  book_print    VARCHAR(64)  NOT NULL DEFAULT '',
  source        VARCHAR(64)  NOT NULL DEFAULT '',
  updated_by    VARCHAR(64)  NOT NULL DEFAULT '',
  revoke_count  INT          NOT NULL DEFAULT 0,
  body          JSON         NOT NULL,
  updated_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_cmg_pair ON cv_mark_gens (from_asset, to_asset);

CREATE TABLE IF NOT EXISTS cv_quote_snaps (
  id          VARCHAR(64) NOT NULL PRIMARY KEY,
  lease_id    VARCHAR(64) NOT NULL,
  user_id     VARCHAR(64) NOT NULL,
  from_asset  VARCHAR(32) NOT NULL,
  to_asset    VARCHAR(32) NOT NULL,
  amount      VARCHAR(64) NOT NULL,
  rate        VARCHAR(64) NOT NULL,
  book        VARCHAR(16) NOT NULL,
  mark_gen    BIGINT      NOT NULL DEFAULT 1,
  expires_at  DATETIME(3) NOT NULL,
  used        TINYINT     NOT NULL DEFAULT 0,
  order_id    VARCHAR(64) NOT NULL DEFAULT '',
  body        JSON        NOT NULL,
  created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE TABLE IF NOT EXISTS cv_quote_proofs (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  lease_id        VARCHAR(64)  NOT NULL,
  quote_id        VARCHAR(64)  NOT NULL,
  user_id         VARCHAR(64)  NOT NULL,
  from_asset      VARCHAR(32)  NOT NULL,
  to_asset        VARCHAR(32)  NOT NULL,
  kind            VARCHAR(32)  NOT NULL,
  lease_mark_gen  BIGINT       NOT NULL DEFAULT 0,
  live_mark_gen   BIGINT       NOT NULL DEFAULT 0,
  lease_rate      VARCHAR(64)  NOT NULL DEFAULT '',
  live_rate       VARCHAR(64)  NOT NULL DEFAULT '',
  lease_book      VARCHAR(16)  NOT NULL DEFAULT '',
  live_book       VARCHAR(16)  NOT NULL DEFAULT '',
  lease_print     VARCHAR(64)  NOT NULL DEFAULT '',
  live_print      VARCHAR(64)  NOT NULL DEFAULT '',
  blocked         TINYINT      NOT NULL DEFAULT 1,
  sealed          TINYINT      NOT NULL DEFAULT 1,
  detail          TEXT         NOT NULL,
  body            JSON         NOT NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);

CREATE INDEX idx_cqp_lease ON cv_quote_proofs (lease_id);
CREATE INDEX idx_cqp_quote ON cv_quote_proofs (quote_id);
CREATE INDEX idx_cqp_user ON cv_quote_proofs (user_id);
CREATE INDEX idx_cqp_kind ON cv_quote_proofs (kind);
// temporary guard removed after the production check
