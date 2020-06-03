-- 对账差异分类处置工作流引擎数据表。
-- 功能：存储差异分类结果、动作定义、流程定义、运行实例、步骤记录。
-- 边界：只写本引擎数据；不修改 aa_recon_tickets 等三方公式表。
-- 不变量：aa_recon_flow_runs.state 枚举须与 reconflow.RunState 对齐。
--
-- 作者：TengLong / MoXinru
-- 日期：2022.03.10

-- ──────────────────────────────────────────────
-- 差异分类结果表
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_recon_diff_classes (
  id               VARCHAR(64)    NOT NULL,
  feature_hash     VARCHAR(32)    NOT NULL,
  kind             VARCHAR(64)    NOT NULL,
  confidence       VARCHAR(16)    NOT NULL,
  score            DECIMAL(6,4)   NOT NULL DEFAULT 0,
  matched_rules_json TEXT         NOT NULL DEFAULT '[]',
  suggested_acts_json TEXT        NOT NULL DEFAULT '[]',
  note             VARCHAR(512)   NOT NULL DEFAULT '',
  classified_at    DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_aa_recon_diff_classes_hash (feature_hash)
);

-- ──────────────────────────────────────────────
-- 动作定义表（自定义动作持久化；内置动作由代码维护）
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_recon_action_defs (
  id               VARCHAR(64)    NOT NULL,
  name             VARCHAR(128)   NOT NULL,
  kind             VARCHAR(64)    NOT NULL,
  risk             VARCHAR(16)    NOT NULL DEFAULT 'low',
  auto_exec        TINYINT(1)     NOT NULL DEFAULT 1,
  require_gate     TINYINT(1)     NOT NULL DEFAULT 0,
  params_json      TEXT           NOT NULL DEFAULT '{}',
  description      VARCHAR(512)   NOT NULL DEFAULT '',
  created_at       DATETIME(3)    NOT NULL,
  updated_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_aa_recon_action_defs_name (name)
);

-- ──────────────────────────────────────────────
-- 流程定义表
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_recon_flow_defs (
  id               VARCHAR(64)    NOT NULL,
  name             VARCHAR(128)   NOT NULL,
  diff_kind        VARCHAR(64)    NOT NULL,
  sla_seconds      BIGINT         NOT NULL DEFAULT 14400,
  auto_classify    TINYINT(1)     NOT NULL DEFAULT 1,
  min_confidence   VARCHAR(16)    NOT NULL DEFAULT 'medium',
  action_ids_json  TEXT           NOT NULL DEFAULT '[]',
  escalate_on_sla  TINYINT(1)     NOT NULL DEFAULT 1,
  request_circuit  TINYINT(1)     NOT NULL DEFAULT 0,
  enabled          TINYINT(1)     NOT NULL DEFAULT 1,
  description      VARCHAR(512)   NOT NULL DEFAULT '',
  created_by       VARCHAR(64)    NOT NULL DEFAULT '',
  created_at       DATETIME(3)    NOT NULL,
  updated_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  KEY idx_aa_recon_flow_defs_kind (diff_kind, enabled)
);

-- ──────────────────────────────────────────────
-- 流程运行实例表
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_recon_flow_runs (
  id               VARCHAR(64)    NOT NULL,
  flow_def_id      VARCHAR(64)    NOT NULL DEFAULT '',
  ticket_id        VARCHAR(64)    NOT NULL DEFAULT '',
  user_id          VARCHAR(64)    NOT NULL DEFAULT '',
  asset            VARCHAR(32)    NOT NULL DEFAULT '',
  chain            VARCHAR(32)    NOT NULL DEFAULT '',
  diff_kind        VARCHAR(64)    NOT NULL DEFAULT '',
  state            VARCHAR(32)    NOT NULL,
  class_id         VARCHAR(64)    NOT NULL DEFAULT '',
  confidence       VARCHAR(16)    NOT NULL DEFAULT '',
  current_step     INT            NOT NULL DEFAULT 0,
  gate_actor       VARCHAR(64)    NOT NULL DEFAULT '',
  gate_at          DATETIME(3)    NULL,
  closed_by        VARCHAR(64)    NOT NULL DEFAULT '',
  escalated_by     VARCHAR(64)    NOT NULL DEFAULT '',
  note             VARCHAR(512)   NOT NULL DEFAULT '',
  fence_epoch      BIGINT         NOT NULL DEFAULT 0,
  fence_token      VARCHAR(64)    NOT NULL DEFAULT '',
  created_by       VARCHAR(64)    NOT NULL DEFAULT '',
  created_at       DATETIME(3)    NOT NULL,
  updated_at       DATETIME(3)    NOT NULL,
  sla_deadline     DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  KEY idx_aa_recon_flow_runs_state    (state, created_at),
  KEY idx_aa_recon_flow_runs_user     (user_id, state),
  KEY idx_aa_recon_flow_runs_ticket   (ticket_id),
  KEY idx_aa_recon_flow_runs_sla      (sla_deadline, state)
);

-- ──────────────────────────────────────────────
-- 流程步骤记录表
-- ──────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS aa_recon_flow_steps (
  id               VARCHAR(64)    NOT NULL,
  run_id           VARCHAR(64)    NOT NULL,
  seq              INT            NOT NULL DEFAULT 0,
  action_def_id    VARCHAR(64)    NOT NULL DEFAULT '',
  action_kind      VARCHAR(64)    NOT NULL DEFAULT '',
  state            VARCHAR(16)    NOT NULL DEFAULT 'pending',
  actor            VARCHAR(64)    NOT NULL DEFAULT '',
  input            TEXT           NOT NULL DEFAULT '',
  output           TEXT           NOT NULL DEFAULT '',
  err_msg          VARCHAR(512)   NOT NULL DEFAULT '',
  gated_by         VARCHAR(64)    NOT NULL DEFAULT '',
  gated_at         DATETIME(3)    NULL,
  started_at       DATETIME(3)    NULL,
  finished_at      DATETIME(3)    NULL,
  created_at       DATETIME(3)    NOT NULL,
  PRIMARY KEY (id),
  KEY idx_aa_recon_flow_steps_run  (run_id, seq),
  KEY idx_aa_recon_flow_steps_gate (state, created_at)
);
