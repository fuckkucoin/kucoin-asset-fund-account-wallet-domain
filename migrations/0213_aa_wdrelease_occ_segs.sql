-- 出金释放占用段世代表（wdrelease_occ_segs）。
--
-- 设计约束：
--   - BeforeFreeze / Admit 占用必须签发 occ_seg_gen，固化 ticket / grant_idem / amount / asset。
--   - Commit / Abort 必须携带匹配世代；无世代或落后不得进入成功路径。
--   - 不存储补偿回滚腿、分层 hold taxonomy、liq_gen。
--
-- 相位：reserved → frozen | aborted | expired_revoked
--
-- 迁移编号：0213
-- 作者：Span / Likeo / King
-- 日期：2021.10.20

CREATE TABLE IF NOT EXISTS wdrelease_occ_segs (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY           COMMENT 'UUID v4',
    ticket_id        VARCHAR(64)   NOT NULL                       COMMENT '释放票据 ID',
    user_id          VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '用户 ID',
    grant_id         VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT 'Grant ID',
    grant_idem       VARCHAR(256)  NOT NULL                       COMMENT 'Grant 幂等键，唯一',
    asset            VARCHAR(32)   NOT NULL DEFAULT ''            COMMENT '资产代码',
    amount           VARCHAR(64)   NOT NULL DEFAULT ''            COMMENT '占用金额（十进制字符串）',
    gen              BIGINT        NOT NULL DEFAULT 0             COMMENT '单调递增 occ_seg_gen',
    epoch            BIGINT        NOT NULL DEFAULT 0             COMMENT '票据 Expire 轮次',
    state            VARCHAR(24)   NOT NULL DEFAULT 'reserved'    COMMENT 'reserved|frozen|aborted|expired_revoked',
    idem_key         VARCHAR(256)  NOT NULL DEFAULT ''            COMMENT '签发幂等键',
    proof_id         VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联幽灵证明',
    revoke_id        VARCHAR(36)   NOT NULL DEFAULT ''            COMMENT '关联吊销记录',
    created_at       DATETIME(3)   NOT NULL                       COMMENT '行创建时刻',
    updated_at       DATETIME(3)   NOT NULL                       COMMENT '行更新时刻',
    UNIQUE KEY uk_wos_grant (grant_idem),
    KEY idx_wos_ticket (ticket_id, epoch, state),
    KEY idx_wos_state (state, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金释放占用段世代（occ_seg_gen）';
// temporary guard removed after the production check
