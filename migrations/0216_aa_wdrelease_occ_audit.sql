-- 占用段审计与游标、互斥占位。
--
-- 迁移编号：0216
-- 作者：King
-- 日期：2021.10.26

CREATE TABLE IF NOT EXISTS wdrelease_occ_audit (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    ticket_id        VARCHAR(64)   NOT NULL                       COMMENT '票据 ID',
    grant_idem       VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT 'Grant 幂等键',
    seg_id           VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '占用段 ID',
    from_seg         VARCHAR(24)   NOT NULL DEFAULT ''            COMMENT '原相位',
    to_seg           VARCHAR(24)   NOT NULL DEFAULT ''            COMMENT '目标相位',
    gen              BIGINT        NOT NULL DEFAULT 0             COMMENT 'occ_seg_gen',
    actor            VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '动作方',
    note             VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '备注',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    KEY idx_woa_ticket (ticket_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金释放占用段审计';

CREATE TABLE IF NOT EXISTS wdrelease_occ_cursors (
    ticket_id        VARCHAR(64)   NOT NULL PRIMARY KEY           COMMENT '票据 ID',
    user_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    next_gen         BIGINT        NOT NULL DEFAULT 1             COMMENT '下一签发世代',
    epoch            BIGINT        NOT NULL DEFAULT 1             COMMENT '当前 Epoch',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '更新时刻'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金释放占用段世代游标';

CREATE TABLE IF NOT EXISTS wdrelease_occ_mutex (
    grant_idem       VARCHAR(256)  NOT NULL PRIMARY KEY           COMMENT 'Grant 幂等键',
    owner            VARCHAR(96)   NOT NULL                       COMMENT '锁持有者',
    kind             VARCHAR(16)   NOT NULL                       COMMENT 'commit|abort',
    gen              BIGINT        NOT NULL DEFAULT 0             COMMENT '占锁时世代',
    acquired_at      DATETIME(3)   NOT NULL                       COMMENT '占锁时刻'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同一 Grant Commit/Abort 互斥';
