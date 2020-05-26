-- 划转半截超时收口与对手方镜像状态机。
-- 包含：half_hops（半截工单）、closeout_plans（收口计划）、closeout_legs（收口腿）、
--        mirror_views（对手方镜像）、mirror_events（镜像审计）、xfer_slas（SLA 规则）。
-- 作者：TianpengLan
-- 不修改 r1 三方对账公式；不改写 fundpath Credit/Egress 核心表。

CREATE TABLE IF NOT EXISTS aa_half_hops (
  id                VARCHAR(64)    NOT NULL PRIMARY KEY,
  hop_id            VARCHAR(191)   NOT NULL,
  transfer_id       VARCHAR(128)   NOT NULL DEFAULT '',
  user_id           VARCHAR(64)    NOT NULL,
  asset             VARCHAR(16)    NOT NULL,
  amount            DECIMAL(36,18) NOT NULL,
  from_book         VARCHAR(64)    NOT NULL,
  to_book           VARCHAR(64)    NOT NULL,
  kind              VARCHAR(32)    NOT NULL,
  state             VARCHAR(32)    NOT NULL,
  sla_breached      TINYINT        NOT NULL DEFAULT 0,
  sla_deadline      VARCHAR(40)    NOT NULL DEFAULT '',
  sla_rule_id       VARCHAR(64)    NOT NULL DEFAULT '',
  closeout_plan_id  VARCHAR(64)    NOT NULL DEFAULT '',
  escalate_reason   VARCHAR(255)   NOT NULL DEFAULT '',
  x_shard           TINYINT        NOT NULL DEFAULT 0,
  detected_at       VARCHAR(40)    NOT NULL,
  updated_at        VARCHAR(40)    NOT NULL,
  closed_at         VARCHAR(40)    NULL,
  UNIQUE KEY uk_aa_half_hops_hop_id (hop_id),
  KEY idx_aa_half_hops_state (state),
  KEY idx_aa_half_hops_user (user_id),
  KEY idx_aa_half_hops_asset (asset),
  KEY idx_aa_half_hops_detected (detected_at)
);

CREATE TABLE IF NOT EXISTS aa_closeout_plans (
  id               VARCHAR(64)  NOT NULL PRIMARY KEY,
  hop_id           VARCHAR(191) NOT NULL,
  half_hop_id      VARCHAR(64)  NOT NULL,
  state            VARCHAR(32)  NOT NULL,
  selected_action  VARCHAR(64)  NOT NULL,
  reason           VARCHAR(255) NOT NULL DEFAULT '',
  actor            VARCHAR(64)  NOT NULL DEFAULT 'system',
  attempts         INT          NOT NULL DEFAULT 0,
  last_error       VARCHAR(512) NOT NULL DEFAULT '',
  abort_reason     VARCHAR(255) NOT NULL DEFAULT '',
  done_at          VARCHAR(40)  NULL,
  created_at       VARCHAR(40)  NOT NULL,
  updated_at       VARCHAR(40)  NOT NULL,
  KEY idx_aa_closeout_plans_hop (hop_id),
  KEY idx_aa_closeout_plans_state (state),
  KEY idx_aa_closeout_plans_created (created_at)
);

CREATE TABLE IF NOT EXISTS aa_closeout_legs (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  plan_id      VARCHAR(64)  NOT NULL,
  hop_id       VARCHAR(191) NOT NULL,
  seq_no       INT          NOT NULL DEFAULT 0,
  action       VARCHAR(64)  NOT NULL,
  state        VARCHAR(32)  NOT NULL,
  idem_key     VARCHAR(191) NOT NULL,
  entry_ref    VARCHAR(128) NOT NULL DEFAULT '',
  note         VARCHAR(255) NOT NULL DEFAULT '',
  attempts     INT          NOT NULL DEFAULT 0,
  started_at   VARCHAR(40)  NULL,
  finished_at  VARCHAR(40)  NULL,
  created_at   VARCHAR(40)  NOT NULL,
  UNIQUE KEY uk_aa_closeout_legs_idem (idem_key),
  KEY idx_aa_closeout_legs_plan (plan_id),
  KEY idx_aa_closeout_legs_hop (hop_id)
);

CREATE TABLE IF NOT EXISTS aa_mirror_views (
  id                    VARCHAR(64)    NOT NULL PRIMARY KEY,
  hop_id                VARCHAR(191)   NOT NULL,
  initiator_user_id     VARCHAR(64)    NOT NULL,
  counterparty_user_id  VARCHAR(64)    NOT NULL DEFAULT '',
  asset                 VARCHAR(16)    NOT NULL,
  amount                DECIMAL(36,18) NOT NULL,
  from_book             VARCHAR(64)    NOT NULL,
  to_book               VARCHAR(64)    NOT NULL,
  state                 VARCHAR(32)    NOT NULL,
  state_reason          VARCHAR(255)   NOT NULL DEFAULT '',
  closeout_plan_id      VARCHAR(64)    NOT NULL DEFAULT '',
  entry_ref             VARCHAR(128)   NOT NULL DEFAULT '',
  version               BIGINT         NOT NULL DEFAULT 0,
  override_actor        VARCHAR(64)    NOT NULL DEFAULT '',
  created_at            VARCHAR(40)    NOT NULL,
  updated_at            VARCHAR(40)    NOT NULL,
  UNIQUE KEY uk_aa_mirror_views_hop (hop_id),
  KEY idx_aa_mirror_views_state (state),
  KEY idx_aa_mirror_views_initiator (initiator_user_id),
  KEY idx_aa_mirror_views_counterparty (counterparty_user_id),
  KEY idx_aa_mirror_views_asset (asset)
);

CREATE TABLE IF NOT EXISTS aa_mirror_events (
  id          VARCHAR(64)  NOT NULL PRIMARY KEY,
  mirror_id   VARCHAR(64)  NOT NULL,
  hop_id      VARCHAR(191) NOT NULL,
  from_state  VARCHAR(32)  NOT NULL,
  to_state    VARCHAR(32)  NOT NULL,
  reason      VARCHAR(255) NOT NULL DEFAULT '',
  actor       VARCHAR(64)  NOT NULL DEFAULT '',
  at          VARCHAR(40)  NOT NULL,
  KEY idx_aa_mirror_events_mirror (mirror_id, at),
  KEY idx_aa_mirror_events_hop (hop_id)
);

CREATE TABLE IF NOT EXISTS aa_xfer_slas (
  id         VARCHAR(64) NOT NULL PRIMARY KEY,
  body       TEXT        NOT NULL,
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL
);
