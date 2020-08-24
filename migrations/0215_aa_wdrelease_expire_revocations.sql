-- Expire 吊销记录（wdrelease_expire_revocations）。
--
-- Expire→Pending/再 Admit 必须吊销旧 occ_seg_gen；旧 Grant 不得再 Commit/Abort。
--
-- 迁移编号：0215
-- 作者：Likeo / King
-- 日期：2021.10.24

CREATE TABLE IF NOT EXISTS wdrelease_expire_revocations (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    ticket_id        VARCHAR(64)   NOT NULL                       COMMENT '票据 ID',
    user_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    from_epoch       BIGINT        NOT NULL DEFAULT 0             COMMENT '被吊销 Epoch',
    to_epoch         BIGINT        NOT NULL DEFAULT 0             COMMENT '吊销后 Epoch 快照',
    revoked_n        INT           NOT NULL DEFAULT 0             COMMENT '吊销占用段条数',
    state            VARCHAR(16)   NOT NULL DEFAULT 'applied'     COMMENT 'applied',
    idem_key         VARCHAR(256)  NOT NULL                       COMMENT 'ticket+epoch 幂等',
    note             VARCHAR(512)  NOT NULL DEFAULT ''            COMMENT '备注',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_wer_idem (idem_key),
    KEY idx_wer_ticket (ticket_id, from_epoch)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金释放 Expire 世代吊销';
