-- leftover 按链|资产 upsert 的门控世代表。
-- 与 0205 elg_egress_generations 历史行并存：本表按 scope_key 覆盖写。
--
-- 作者：Axel
-- 迁移编号：0225

CREATE TABLE IF NOT EXISTS elg_egress_generation_scopes (
    scope_key     VARCHAR(80)  NOT NULL PRIMARY KEY           COMMENT 'CHAIN|ASSET',
    chain         VARCHAR(32)  NOT NULL DEFAULT ''            COMMENT '链标识',
    asset         VARCHAR(32)  NOT NULL DEFAULT ''            COMMENT '资产代码',
    generation    BIGINT       NOT NULL DEFAULT 0             COMMENT '当前世代',
    flags_fp      VARCHAR(64)  NOT NULL DEFAULT ''            COMMENT '停仓指纹',
    breach_level  VARCHAR(16)  NOT NULL DEFAULT ''            COMMENT '水位档',
    tank_digest   VARCHAR(64)  NOT NULL DEFAULT ''            COMMENT '油箱摘要',
    decision      VARCHAR(16)  NOT NULL DEFAULT ''            COMMENT '门控结论',
    reason        VARCHAR(64)  NOT NULL DEFAULT ''            COMMENT '推进原因',
    updated_at    DATETIME(3)  NOT NULL                       COMMENT '更新时刻',
    KEY idx_elg_scope_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出金门控按范围世代（leftover upsert）';
