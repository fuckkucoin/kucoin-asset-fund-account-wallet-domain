-- 账户关户世代（account_close_epochs）。
--
-- 设计约束：
--   - 关户成功必须升降 close_epoch；重开同样升世代。
--   - 贷记路径必须绑定当前世代；无世代或失配不得成功。
--   - 不存储地址池 recycle_epoch、Memo 工单、生命周期 hop。
--
-- 相位：open → closing → closed / reopened
--
-- 迁移编号：0218
-- 作者：Span / Likeo / King
-- 日期：2021.07.06

CREATE TABLE IF NOT EXISTS account_close_epochs (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id          VARCHAR(64)   NOT NULL                       COMMENT '用户 ID',
    book             VARCHAR(32)   NOT NULL                       COMMENT '科目',
    close_epoch      BIGINT        NOT NULL DEFAULT 0             COMMENT '当前 close_epoch',
    state            VARCHAR(24)   NOT NULL DEFAULT 'open'        COMMENT 'open|closing|closed|reopened',
    closed_epoch     BIGINT        NOT NULL DEFAULT 0             COMMENT '最近一次关户世代',
    live_epoch       BIGINT        NOT NULL DEFAULT 0             COMMENT '当前活户世代',
    close_reason     VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '关户原因',
    reopen_reason    VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '重开原因',
    idem_key         VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '幂等键',
    actor            VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作者',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_ace_user_book (user_id, book),
    KEY idx_ace_state (state, updated_at),
    KEY idx_ace_epoch (close_epoch)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账户关户世代 close_epoch';

CREATE TABLE IF NOT EXISTS account_close_epoch_cursors (
    user_id          VARCHAR(64)   NOT NULL                       COMMENT '用户 ID',
    book             VARCHAR(32)   NOT NULL                       COMMENT '科目',
    next_epoch       BIGINT        NOT NULL DEFAULT 1             COMMENT '下一世代',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '更新时刻',
    PRIMARY KEY (user_id, book)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账户关户世代游标';

CREATE TABLE IF NOT EXISTS account_close_epoch_audit (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    user_id          VARCHAR(64)   NOT NULL                       COMMENT '用户 ID',
    book             VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '科目',
    from_state       VARCHAR(24)   NOT NULL DEFAULT ''            COMMENT '原相位',
    to_state         VARCHAR(24)   NOT NULL DEFAULT ''            COMMENT '目标相位',
    from_epoch       BIGINT        NOT NULL DEFAULT 0             COMMENT '原世代',
    to_epoch         BIGINT        NOT NULL DEFAULT 0             COMMENT '目标世代',
    actor            VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '操作者',
    note             VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '备注',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '记录时刻',
    KEY idx_acea_user (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账户关户世代审计';
