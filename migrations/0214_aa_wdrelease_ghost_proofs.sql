-- 幽灵占用证明仓（wdrelease_ghost_proofs）。
--
-- 覆盖：Commit↔Abort 夹缝、世代落后、缺世代、Expire 重开旧 Grant、活链解冻拒。
-- 状态：open → acked
--
-- 迁移编号：0214
-- 作者：Span / Likeo
-- 日期：2021.10.22

CREATE TABLE IF NOT EXISTS wdrelease_ghost_proofs (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    kind             VARCHAR(24)   NOT NULL DEFAULT 'ghost'       COMMENT 'ghost|race|stale|missing|expire|live_unfreeze',
    ticket_id        VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '票据 ID',
    user_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    grant_idem       VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT 'Grant 幂等键',
    seg_id           VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '占用段 ID',
    expected_gen     BIGINT        NOT NULL DEFAULT 0             COMMENT '调用方世代',
    actual_gen       BIGINT        NOT NULL DEFAULT 0             COMMENT '仓内世代',
    reason           VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT '夹缝原因码',
    state            VARCHAR(16)   NOT NULL DEFAULT 'open'        COMMENT 'open|acked',
    idem_key         VARCHAR(256)  NOT NULL                       COMMENT '证明幂等',
    digest           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'SHA-256 十六进制',
    ledger_frozen    TINYINT       NOT NULL DEFAULT 0             COMMENT '账本已冻',
    abort_would_ok   TINYINT       NOT NULL DEFAULT 0             COMMENT '若放行 Abort 会成功',
    phase_at_issue   VARCHAR(24)   NOT NULL DEFAULT ''            COMMENT '签发时占用段相位',
    actor            VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '动作方',
    note             VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '备注',
    acked_at         DATETIME(3)   NULL                           COMMENT 'acked 时刻',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_wgp_idem (idem_key),
    KEY idx_wgp_ticket (ticket_id, state),
    KEY idx_wgp_kind (kind, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金释放幽灵占用证明';
