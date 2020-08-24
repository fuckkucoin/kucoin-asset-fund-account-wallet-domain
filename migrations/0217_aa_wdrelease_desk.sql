-- 出金释放台票据与 Grant（wdrelease 深扩，供 occ_seg_gen 只读挂钩）。
--
-- 不存储补偿腿、分层 hold、liq_gen。
--
-- 迁移编号：0217
-- 作者：Span
-- 日期：2021.10.28

CREATE TABLE IF NOT EXISTS wdrelease_tickets (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id          VARCHAR(64)   NOT NULL                       COMMENT '用户 ID',
    asset            VARCHAR(32)   NOT NULL                       COMMENT '资产代码',
    amount           VARCHAR(64)   NOT NULL                       COMMENT '金额（十进制字符串）',
    state            VARCHAR(16)   NOT NULL DEFAULT 'pending'     COMMENT 'pending|admitted|reserved|frozen|aborted|expired',
    epoch            BIGINT        NOT NULL DEFAULT 1             COMMENT 'Expire 重开轮次',
    grant_id         VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '当前 Grant',
    grant_idem       VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '当前 Grant 幂等',
    note             VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '备注',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '更新时刻',
    KEY idx_wrt_user (user_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金释放票据';

CREATE TABLE IF NOT EXISTS wdrelease_grants (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    ticket_id        VARCHAR(64)   NOT NULL                       COMMENT '票据 ID',
    user_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    grant_idem       VARCHAR(256)  NOT NULL                       COMMENT '幂等键',
    asset            VARCHAR(32)   NOT NULL                       COMMENT '资产',
    amount           VARCHAR(64)   NOT NULL                       COMMENT '占用金额',
    state            VARCHAR(16)   NOT NULL DEFAULT 'reserved'    COMMENT 'issued|reserved|frozen|aborted|expired|revoked',
    epoch            BIGINT        NOT NULL DEFAULT 1             COMMENT '票据 Epoch',
    ledger_tag       VARCHAR(16)   NOT NULL DEFAULT 'reserved'    COMMENT 'reserved|frozen|aborted',
    note             VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '备注',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '更新时刻',
    UNIQUE KEY uk_wrg_idem (grant_idem),
    KEY idx_wrg_ticket (ticket_id, epoch)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金释放占用 Grant';

CREATE TABLE IF NOT EXISTS wdrelease_desk_audit (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    ticket_id        VARCHAR(64)   NOT NULL                       COMMENT '票据 ID',
    grant_id         VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT 'Grant ID',
    from_state       VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT '原状态',
    to_state         VARCHAR(40)   NOT NULL DEFAULT ''            COMMENT '目标状态',
    actor            VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '动作方',
    note             VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '备注',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '创建时刻',
    KEY idx_wda_ticket (ticket_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金释放台审计';
