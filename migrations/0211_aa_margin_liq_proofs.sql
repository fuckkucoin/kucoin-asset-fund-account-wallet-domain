-- 杠杆强平半截与竞态证明仓（margin_liq_proofs）。
--
-- 种类：
--   half — freeze hook 失败或半截超时，借款不得按已完全强平释放
--   race — 世代漂移、缺世代、互斥占用、重复释放
--
-- 状态：open → proven → closed；或 dismissed
--
-- 迁移编号：0211
-- 作者：King
-- 日期：2021.11.10

CREATE TABLE IF NOT EXISTS margin_liq_proofs (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    kind             VARCHAR(16)   NOT NULL                       COMMENT 'half|race',
    borrow_id        VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '借款 ID',
    user_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    lease_id         VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联租约',
    liq_gen          BIGINT        NOT NULL DEFAULT 0             COMMENT '请求世代或半截世代',
    peer_gen         BIGINT        NOT NULL DEFAULT 0             COMMENT '仓位实际世代',
    reason           VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT 'stale_gen / missing_gen / mutex_held / hook_failed 等',
    state            VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open|proven|closed|dismissed',
    idem_key         VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '证明幂等',
    digest           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'sha256 摘要',
    hook_error       VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT 'hook 失败原文',
    phase_at_issue   VARCHAR(16)   NOT NULL DEFAULT ''            COMMENT '落仓时相位',
    actor            VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT 'freeze / release / close / discover',
    proven_at        DATETIME(3)                                  COMMENT '固化时刻',
    closed_at        DATETIME(3)                                  COMMENT '关闭时刻',
    note             VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '运维备注',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '创建',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '更新',
    UNIQUE KEY uk_mlp_idem (idem_key),
    KEY idx_mlp_borrow (borrow_id, kind, state),
    KEY idx_mlp_state (state, created_at),
    KEY idx_mlp_kind (kind, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='杠杆强平半截与竞态证明';
